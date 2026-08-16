package agent

import (
	"context"
	"strings"
	"sync"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/provider"
)

// The static tables will never know every tool, and each one that falls through
// reads as "might write", blocking work that only reads. This is the escalation
// path: asked only when the tables come up short, cached so a command is asked
// once, and never a widening of what may run — the OS sandbox still bounds
// every write regardless of the answer.
const commandClassSystemPrompt = `You classify one shell command as read-only or not.

READONLY: running it cannot create, modify, or delete any file, and cannot change
process or system state. Reading files and printing to stdout are read-only, no
matter how much it reads or prints.

WRITES: it can write, delete, or change permissions on a file; install something;
start or kill a process; or run code handed to it as an argument.

Judge the command in front of you, not the program in general. Most inspection
programs read by default and only write when a flag or construct says so:
-i/--in-place, -o/--output, -w, --fix, --write, a > redirection, or code passed
inline (-c, -e, sed's s///e, awk's print > "file", find -exec).

Answer with exactly one word: READONLY or WRITES.
If you cannot tell what it would do, answer WRITES.`

const (
	// A classification is one word, but a thinking model spends its budget on
	// reasoning first and answers nothing at all under a tight cap — so the call
	// asks for thinking off and still leaves room for a short reply.
	commandClassEffort       = "disabled"
	commandClassMaxOutTokens = 64
	commandClassTimeout      = 20 * time.Second
	commandClassMaxCommand   = 400
)

// commandClassCache remembers one verdict per segment for the process. The
// evidence ledger is audited, so the same segment must classify the same way
// every time it appears; a cached answer is what makes that true.
type commandClassCache struct {
	mu sync.RWMutex
	by map[string]bool
}

func (c *commandClassCache) get(key string) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.by[key]
	return v, ok
}

func (c *commandClassCache) put(key string, readOnly bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.by == nil {
		c.by = map[string]bool{}
	}
	c.by[key] = readOnly
}

var sharedCommandClass commandClassCache

// segmentIsReadOnly reports whether a segment the static tables could not
// classify only reads. The caller supplies the segment, so this serves every
// table that runs short rather than one. It answers false — the conservative
// direction — for anything it cannot establish: no segment, no provider, a
// timeout, or a reply that is not one of the two words.
func (a *Agent) segmentIsReadOnly(ctx context.Context, segment string) bool {
	if a == nil || a.triageProvider() == nil {
		return false
	}
	segment = strings.TrimSpace(segment)
	if segment == "" || len(segment) > commandClassMaxCommand {
		return false
	}
	if verdict, ok := sharedCommandClass.get(segment); ok {
		return verdict
	}
	verdict := a.askCommandClass(ctx, segment)
	sharedCommandClass.put(segment, verdict)
	return verdict
}

// mixedShapeClearedByEscalation asks about the segment that made a command look
// like a mutation beside a verification. Static analysis stays the fast path —
// this runs only when it would otherwise refuse — so an unrecognized read-only
// tool costs one classification, not a block. Delivery never escalates: there
// the shape is refused for what it does to the receipt, not for what it writes.
func (a *Agent) mixedShapeClearedByEscalation(ctx context.Context, plan *toolCallPlan) bool {
	if a == nil || a.deliveryProfile || plan == nil {
		return false
	}
	segment, unproven := evidence.UnprovenSegment(bashCommandFromArgs(plan.evidenceArgs))
	if !unproven {
		return false
	}
	return a.segmentIsReadOnly(ctx, segment)
}

// triageProvider is the model configured for host classifications, and nothing
// else. Borrowing the turn's own provider would put extra calls into a stream
// the run is mid-conversation on — and would bill the main tier for a two-word
// answer — so an unconfigured triage model simply leaves the static verdict
// standing.
func (a *Agent) triageProvider() provider.Provider {
	if a == nil {
		return nil
	}
	return a.svc.triage
}

func (a *Agent) askCommandClass(ctx context.Context, segment string) bool {
	ctx, cancel := context.WithTimeout(ctx, commandClassTimeout)
	defer cancel()

	var usage *provider.Usage
	defer func() {
		if usage != nil && usage.TotalTokens > 0 {
			a.svc.sink.Emit(event.Event{Kind: event.Usage, ModelRef: a.modelRef, Usage: usage,
				Pricing: a.svc.pricing, UsageSource: event.UsageSourceCompaction})
		}
	}()

	ch, err := a.triageProvider().Stream(ctx, provider.Request{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: commandClassSystemPrompt},
			{Role: provider.RoleUser, Content: segment},
		},
		MaxTokens:      commandClassMaxOutTokens,
		EffortOverride: commandClassEffort,
		Temperature:    provider.OptionalTemperature(a.temperature),
	})
	if err != nil {
		return false
	}
	var reply strings.Builder
	for chunk := range ch {
		switch chunk.Type {
		case provider.ChunkText:
			reply.WriteString(chunk.Text)
		case provider.ChunkUsage:
			usage = chunk.Usage
		case provider.ChunkError:
			return false
		}
	}
	return parseCommandClass(reply.String())
}

// parseCommandClass reads the verdict defensively: only a bare READONLY counts,
// so a hedged, chatty, or truncated reply lands on the safe side.
func parseCommandClass(reply string) bool {
	fields := strings.Fields(strings.ToUpper(reply))
	return len(fields) == 1 && fields[0] == "READONLY"
}
