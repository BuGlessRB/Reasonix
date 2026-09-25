package agent

import (
	"bytes"
	"fmt"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

// abortOnPerseveration builds the terminal for a stream the guard cut short. It
// preserves what was produced and marks the result so the turn settles with a
// warning instead of retrying.
func abortOnPerseveration(collect func(string, error) streamedTurn, finishReasoning func() (string, string)) streamedTurn {
	stored, _ := finishReasoning()
	out := collect(stored, nil)
	out.perseverationAborted = true
	return out
}

// perseverationRetryMessage is the host nudge appended before a retry. It is
// not the previous prompt verbatim — resending that would reproduce the loop —
// and it tells the user why the attempt restarted. attempt is the
// consecutive-retry ordinal (1 for the first retry).
func perseverationRetryMessage(attempt int) string {
	return fmt.Sprintf("\n[retrying (%d) avoiding perseveration]\n", attempt)
}

// perseverationLoopNotice is the fallback English text for the
// perseveration_loop notice; a frontend localizes it by the code.
func perseverationLoopNotice() string {
	return "The assistant got stuck repeating the same text; the response was cut short. Try again, add guidance, or switch provider/model."
}

// defaultPerseverationRetries is the nudge-and-retry budget when the caller
// does not set Options.MaxPerseverationRetries.
const defaultPerseverationRetries = 1

// Loop-unit detection bounds. These two plus loopRepeats fully determine
// sensitivity: a unit larger than maxLoopPeriod is invisible to the periodicity
// scan, and a run shorter than minLoopPeriod*loopRepeats can never clear the
// repeat floor — so the smallest positive is 1 KiB of byte-identical repetition.
const (
	minLoopPeriod = 128
	maxLoopPeriod = 2048
	loopRepeats   = 8
)

// scanIntervalBytes amortises the O(maxLoopPeriod) periodicity scan: observe
// runs it at most once per this many appended bytes (trimming the tail on the
// same cadence) instead of on every streamed delta. One minimum span, so the
// first scan lands exactly when the smallest positive can first exist.
const scanIntervalBytes = minLoopPeriod * loopRepeats

// resolvePerseverationRetries maps the optional option onto the effective retry
// budget: nil keeps the default, and a negative value is clamped to 0 (no retry).
func resolvePerseverationRetries(configured *int) int {
	if configured == nil {
		return defaultPerseverationRetries
	}
	return max(*configured, 0)
}

// handlePerseverationAbort decides what happens after the guard ends a stream:
// below the retry budget it appends the nudge and continues the loop, and once
// the budget is spent it stops for the user instead of retrying into the same
// trap. cont=true keeps the tool loop running.
func (a *Agent) handlePerseverationAbort() (cont bool, err error) {
	if a.sess.perseverationStrikes < a.perseverationMaxRetries {
		a.sess.perseverationStrikes++
		a.sess.conversation.Add(provider.Message{Role: provider.RoleUser,
			Content: a.withTurnPreferences(perseverationRetryMessage(a.sess.perseverationStrikes))})
		return true, nil
	}
	a.svc.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn,
		Code: event.NoticeCodePerseverationLoop, Text: perseverationLoopNotice()})
	return false, nil
}

// perseverationGuard detects perseveration (= mindless repetition): the model
// emits a short block of text or reasoning over and over without calling a tool
// or stopping, which no other guard observes — tool-round guards need tool
// calls, and the liveness watchdog only measures silence. It watches the
// rolling tail of one prose channel and fires once on byte-identical repeats.
type perseverationGuard struct {
	tail         []byte
	window       int // bytes of history examined for periodicity
	scanInterval int // appended bytes between periodicity scans (amortisation)
	sinceScan    int // appended bytes since the last scan
	minPeriod    int // shortest block that can count as a loop unit
	maxPeriod    int // longest block that can count as a loop unit
	minRepeats   int // identical repeats required before firing
	fired        bool
}

func newPerseverationGuard() *perseverationGuard {
	return &perseverationGuard{
		// Exactly loopRepeats full maxLoopPeriod blocks: the scan can only reach
		// maxLoopPeriod once the tail is this long, so a smaller window would
		// silently make the largest unit undetectable.
		window:       maxLoopPeriod * loopRepeats,
		scanInterval: scanIntervalBytes,
		minPeriod:    minLoopPeriod,
		maxPeriod:    maxLoopPeriod,
		minRepeats:   loopRepeats,
	}
}

// perseverationGuards holds one guard per prose channel. Reasoning and answer
// deltas are judged on separate buffers so answer text interleaved between
// thinking deltas cannot break their periodicity and mask a thinking loop.
type perseverationGuards struct {
	reasoning *perseverationGuard
	text      *perseverationGuard
}

func newPerseverationGuards() perseverationGuards {
	return perseverationGuards{reasoning: newPerseverationGuard(), text: newPerseverationGuard()}
}

// forChunk returns the guard watching chunk's prose channel, or nil for chunk
// types that carry no prose (tool calls, usage, control).
func (g perseverationGuards) forChunk(t provider.ChunkType) *perseverationGuard {
	switch t {
	case provider.ChunkReasoning:
		return g.reasoning
	case provider.ChunkText:
		return g.text
	default:
		return nil
	}
}

// observe appends one streamed delta and reports whether the accumulated tail
// is now a short block repeated enough times to be degenerate. It reports true
// at most once per stream. The periodicity scan and the tail trim both run on
// the scanInterval cadence, so the O(window) copy is paid per interval.
func (g *perseverationGuard) observe(delta string) bool {
	if g == nil || g.fired || delta == "" {
		return false
	}
	g.tail = append(g.tail, delta...)
	if len(g.tail) > g.window+g.scanInterval {
		g.tail = append(g.tail[:0], g.tail[len(g.tail)-g.window:]...)
	}
	g.sinceScan += len(delta)
	if g.sinceScan < g.scanInterval {
		return false
	}
	g.sinceScan = 0
	if g.degenerate() {
		g.fired = true
		return true
	}
	return false
}

// degenerate reports whether the tail ends with the same block repeated at
// least minRepeats times for some period in [minPeriod, maxPeriod]. The smallest
// qualifying period wins; a unit whose own fundamental period is below
// minPeriod is still caught, at its smallest multiple that clears minPeriod.
func (g *perseverationGuard) degenerate() bool {
	n := len(g.tail)
	limit := min(g.maxPeriod, n/g.minRepeats)
	for period := g.minPeriod; period <= limit; period++ {
		if g.trailingRepeats(period) < g.minRepeats {
			continue
		}
		if meaningfulPerseveration(g.tail[n-period:]) {
			return true
		}
	}
	return false
}

// trailingRepeats counts how many times the final period-length block repeats
// contiguously at the end of the tail.
func (g *perseverationGuard) trailingRepeats(period int) int {
	n := len(g.tail)
	block := g.tail[n-period:]
	count := 1
	for i := n - 2*period; i >= 0; i -= period {
		if !bytes.Equal(g.tail[i:i+period], block) {
			break
		}
		count++
	}
	return count
}

// meaningfulPerseveration filters out blocks that would false-positive on
// legitimate output: whitespace runs, alignment padding, or a single repeated
// character. A real degenerate loop repeats words and punctuation, so require
// some non-space bytes and at least two letters or digits.
func meaningfulPerseveration(block []byte) bool {
	nonSpace, alnum := 0, 0
	for _, b := range block {
		switch {
		case b == ' ' || b == '\t' || b == '\r' || b == '\n':
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', b >= 0x80:
			nonSpace++
			alnum++
		default:
			nonSpace++
		}
	}
	return nonSpace >= 4 && alnum >= 2
}
