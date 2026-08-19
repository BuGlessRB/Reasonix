package hook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Runner binds a set of resolved hooks to a session: a working directory, the
// spawner, and a notify callback that surfaces non-blocking hook messages to the
// user. It is the single object the agent (tool events) and the controller
// (prompt/stop events) fire hooks through, so neither has to know how hooks load
// or run. A nil *Runner is a valid no-op (no hooks configured).
type Runner struct {
	hooks     []ResolvedHook
	cwd       string
	spawner   Spawner
	notify    func(string) // surface a non-blocking (warn/error) hook message; may be nil
	mu        sync.RWMutex
	sessionID string
	// lastOutcome holds the message each hook reported last, so a repeat of the
	// same one is not surfaced again. Keyed by hook, so it is bounded by the
	// configured hook count rather than by how often they run.
	lastOutcome map[string]string
}

// SetSessionID updates the Claude-compatible session identifier used in hook
// payloads. It is safe to call when a controller rotates sessions.
func (r *Runner) SetSessionID(id string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.sessionID = id
	r.mu.Unlock()
}

func (r *Runner) payload(event Event) Payload {
	r.mu.RLock()
	id := r.sessionID
	r.mu.RUnlock()
	return Payload{Event: event, Cwd: r.cwd, SessionID: id}
}

// NewRunner builds a Runner. spawner nil uses DefaultSpawner; notify nil drops
// non-blocking messages.
func NewRunner(hooks []ResolvedHook, cwd string, spawner Spawner, notify func(string)) *Runner {
	return &Runner{hooks: hooks, cwd: cwd, spawner: spawner, notify: notify}
}

// Hooks returns the resolved hooks (for `/hooks` listing).
func (r *Runner) Hooks() []ResolvedHook { return r.snapshot() }

// Replace swaps the hook set a live session fires. Editing hooks has to reach
// the open session: a rule the user just fixed that only applies after a restart
// is indistinguishable from one that does not work.
func (r *Runner) Replace(hooks []ResolvedHook) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.hooks = hooks
	// Edited hooks report from a clean slate: a message suppressed as a repeat
	// of the old configuration says something new about the new one.
	r.lastOutcome = nil
	r.mu.Unlock()
}

// Spawner is the interpreter binding this session's hooks already run under, so
// a dry run resolves the same bash the real invocation would.
func (r *Runner) Spawner() Spawner {
	if r == nil {
		return nil
	}
	return r.spawner
}

// snapshot reads the hook set under the lock, since Replace may run while a turn
// is firing hooks.
func (r *Runner) snapshot() []ResolvedHook {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hooks
}

// Enabled reports whether any hooks are configured.
func (r *Runner) Enabled() bool { return len(r.snapshot()) > 0 }

// Has reports whether any configured hook listens for the given event. Callers
// use it to skip work that only matters when a specific hook exists (e.g. the
// agent buffers reasoning for transform only when a PostLLMCall hook is set).
func (r *Runner) Has(event Event) bool {
	if r == nil {
		return false
	}
	for _, h := range r.snapshot() {
		if h.Event == event {
			return true
		}
	}
	return false
}

// HasPostLLMCall reports whether a PostLLMCall hook is configured, so the agent
// keeps streaming reasoning live unless a transform is actually wired up.
func (r *Runner) HasPostLLMCall() bool { return r.Has(PostLLMCall) }

// ToolMutationHooksEnabled reports whether any hook runs around a tool call.
// These hooks execute user shell code and may mutate paths that the tool itself
// does not declare, so checkpoint coverage must account for them.
func (r *Runner) ToolMutationHooksEnabled() bool {
	return r.Has(PreToolUse) || r.Has(PostToolUse) || r.Has(PostToolUseFailure)
}

// PreToolUse fires before a tool call. block=true means the call must be
// refused; message is the reason (fed back to the model and shown to the user).
func (r *Runner) PreToolUse(ctx context.Context, name string, args json.RawMessage) (block bool, message string) {
	if !r.Enabled() {
		return false, ""
	}
	p := r.payload(PreToolUse)
	p.ToolName, p.ToolArgs = name, args
	rep := Run(ctx, p, r.snapshot(), r.spawner)
	return r.handle(rep)
}

// PostToolUse fires after a tool call. It can't block; non-pass outcomes are
// surfaced to the user via notify.
func (r *Runner) PostToolUse(ctx context.Context, name string, args json.RawMessage, result string) {
	if !r.Enabled() {
		return
	}
	p := r.payload(PostToolUse)
	p.ToolName, p.ToolArgs, p.ToolResult = name, args, result
	rep := Run(ctx, p, r.snapshot(), r.spawner)
	r.handle(rep)
}

// PostToolUseFailure fires when a tool invocation returns an error.
func (r *Runner) PostToolUseFailure(ctx context.Context, name string, args json.RawMessage, result string, err error) {
	if !r.Enabled() {
		return
	}
	p := r.payload(PostToolUseFailure)
	p.ToolName, p.ToolArgs, p.ToolResult = name, args, result
	if err != nil {
		p.Error = err.Error()
		p.IsInterrupt = errors.Is(err, context.Canceled)
	}
	r.handle(Run(ctx, p, r.snapshot(), r.spawner))
	// Native Reasonix PostToolUse historically observed both success and
	// failure. Preserve that contract while Claude hooks use the distinct event.
	legacy := r.nativeHooks(PostToolUse)
	if len(legacy) > 0 {
		p.Event = PostToolUse
		r.handle(Run(ctx, p, legacy, r.spawner))
	}
}

// PermissionRequest fires before a tool approval prompt is shown. A native
// Reasonix hook here can't answer the dialog (non-pass outcomes are surfaced
// via notify only); a Claude-imported hook (PayloadFormat "claude") can
// answer it on the user's behalf via exit 2 or a JSON decision, matching
// Claude's own contract. decision == nil means "no opinion, show the prompt
// normally"; a non-nil decision means the caller should skip the prompt and
// treat it as denied (false) or auto-approved (true).
func (r *Runner) PermissionRequest(ctx context.Context, name, subject string, args json.RawMessage) (decision *bool, message string) {
	if !r.Enabled() {
		return nil, ""
	}
	p := r.payload(PermissionRequest)
	p.ToolName, p.ToolArgs, p.Subject = name, args, subject
	rep := Run(ctx, p, r.snapshot(), r.spawner)
	block, msg := r.handle(rep)
	switch {
	case block:
		deny := false
		return &deny, msg
	case rep.Allowed:
		allow := true
		return &allow, msg
	default:
		return nil, msg
	}
}

// PromptSubmit fires before a turn starts. block=true aborts the turn; message
// is the reason.
func (r *Runner) PromptSubmit(ctx context.Context, prompt string, turn int) (block bool, message string) {
	if !r.Enabled() {
		return false, ""
	}
	p := r.payload(UserPromptSubmit)
	p.Prompt, p.Turn = prompt, turn
	rep := Run(ctx, p, r.snapshot(), r.spawner)
	return r.handle(rep)
}

// Stop fires after a turn finishes. It can't block.
func (r *Runner) Stop(ctx context.Context, lastAssistant string, turn int) {
	if !r.Enabled() {
		return
	}
	p := r.payload(Stop)
	p.LastAssistant, p.Turn = lastAssistant, turn
	rep := Run(ctx, p, r.snapshot(), r.spawner)
	r.handle(rep)
}

// StopResult emits Stop on success and StopFailure when the turn failed.
func (r *Runner) StopResult(ctx context.Context, lastAssistant string, turn int, err error) {
	if err == nil {
		r.Stop(ctx, lastAssistant, turn)
		return
	}
	if !r.Enabled() {
		return
	}
	p := r.payload(StopFailure)
	p.LastAssistant, p.Turn, p.Error = lastAssistant, turn, err.Error()
	p.IsInterrupt = errors.Is(err, context.Canceled)
	r.handle(Run(ctx, p, r.snapshot(), r.spawner))
	legacy := r.nativeHooks(Stop)
	if len(legacy) > 0 {
		p.Event = Stop
		r.handle(Run(ctx, p, legacy, r.spawner))
	}
}

func (r *Runner) nativeHooks(event Event) []ResolvedHook {
	var out []ResolvedHook
	for _, h := range r.snapshot() {
		if h.Event == event && h.PayloadFormat != "claude" {
			out = append(out, h)
		}
	}
	return out
}

// SessionStart fires when a session becomes active. It can't block; successful
// stdout may contribute one-shot context for the next model request.
func (r *Runner) SessionStart(ctx context.Context, source ...string) []string {
	if !r.Enabled() {
		return nil
	}
	p := r.payload(SessionStart)
	p.Source = "startup"
	if len(source) > 0 && strings.TrimSpace(source[0]) != "" {
		p.Source = strings.TrimSpace(source[0])
	}
	rep := Run(ctx, p, r.snapshot(), r.spawner)
	r.handle(rep)
	return r.additionalContexts(rep)
}

// SessionEnd fires when a session is closed or rotated (/new). It can't block.
func (r *Runner) SessionEnd(ctx context.Context, reason ...string) {
	if !r.Enabled() {
		return
	}
	p := r.payload(SessionEnd)
	p.Reason = "other"
	if len(reason) > 0 && strings.TrimSpace(reason[0]) != "" {
		p.Reason = strings.TrimSpace(reason[0])
	}
	r.handle(Run(ctx, p, r.snapshot(), r.spawner))
}

// SubagentStop fires when a `task` sub-agent finishes. It can't block; last is
// the sub-agent's final answer.
func (r *Runner) SubagentStop(ctx context.Context, last string) {
	if !r.Enabled() {
		return
	}
	p := r.payload(SubagentStop)
	p.LastAssistant = last
	r.handle(Run(ctx, p, r.snapshot(), r.spawner))
}

// Notification fires when the agent needs the user's attention (e.g. a pending
// approval). It can't block; message describes what's waiting.
func (r *Runner) Notification(ctx context.Context, message string, notificationType ...string) {
	if !r.Enabled() {
		return
	}
	p := r.payload(Notification)
	p.Message = message
	if len(notificationType) > 0 {
		p.NotificationType = strings.TrimSpace(notificationType[0])
	}
	r.handle(Run(ctx, p, r.snapshot(), r.spawner))
}

// PostLLMCall fires after every model turn completes but before the
// reasoning_content is stored in the session. It returns the hook's stdout as
// the new reasoning text, or the original reasoning if the hook passes with
// empty stdout / doesn't exist / fails. A non-pass outcome is surfaced via
// notify but doesn't block.
func (r *Runner) PostLLMCall(ctx context.Context, reasoning string, turn int) string {
	if !r.Has(PostLLMCall) {
		return reasoning
	}
	p := r.payload(PostLLMCall)
	p.Reasoning, p.Turn = reasoning, turn
	rep := Run(ctx, p, r.snapshot(), r.spawner)
	r.handle(rep)
	for _, o := range rep.Outcomes {
		if o.Decision == DecisionPass {
			if s := strings.TrimSpace(o.Stdout); s != "" {
				return s
			}
		}
	}
	return reasoning
}

// PreCompact fires just before a compaction pass and returns the concatenated
// stdout of its hooks as extra summary guidance, so a hook can steer what the
// summary keeps. Non-pass outcomes are surfaced via notify.
func (r *Runner) PreCompact(ctx context.Context, trigger string) string {
	if !r.Enabled() {
		return ""
	}
	p := r.payload(PreCompact)
	p.Trigger = trigger
	rep := Run(ctx, p, r.snapshot(), r.spawner)
	r.handle(rep)
	var b strings.Builder
	for _, o := range rep.Outcomes {
		if s := strings.TrimSpace(o.Stdout); s != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(s)
		}
	}
	return b.String()
}

func (r *Runner) additionalContexts(rep Report) []string {
	var contexts []string
	for _, o := range rep.Outcomes {
		if o.Decision != DecisionPass {
			continue
		}
		out, warnings := ParseOutput(rep.Event, o.Stdout)
		for _, warning := range warnings {
			if r.notify != nil {
				r.notify(FormatOutcome(Outcome{
					Hook:     o.Hook,
					Decision: DecisionWarn,
					Stdout:   warning,
				}))
			}
		}
		if out.AdditionalContext != "" {
			contexts = append(contexts, out.AdditionalContext)
		}
	}
	return contexts
}

// handle surfaces every non-pass outcome to the user (notify) and returns the
// block decision plus the blocking hook's message.
func (r *Runner) handle(rep Report) (bool, string) {
	var blockMsg string
	for _, o := range rep.Outcomes {
		if o.Decision == DecisionPass {
			continue
		}
		msg := FormatOutcome(o)
		// A block always surfaces: it is how the user learns why the turn stopped.
		// A repeat does not — a hook broken on this host says the same thing
		// before every tool call, and the second copy adds nothing.
		if r.notify != nil && (o.Decision == DecisionBlock || r.outcomeChanged(o, msg)) {
			r.notify(msg)
		}
		if o.Decision == DecisionBlock {
			blockMsg = msg
		}
	}
	return rep.Blocked, blockMsg
}

// outcomeChanged reports whether this hook is saying something different from
// what it said last time, recording the message either way.
func (r *Runner) outcomeChanged(o Outcome, msg string) bool {
	key := strings.Join([]string{string(o.Hook.Scope), string(o.Hook.Event), o.Hook.Source, o.Hook.Command}, "\x00")
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastOutcome == nil {
		r.lastOutcome = map[string]string{}
	}
	previous, seen := r.lastOutcome[key]
	r.lastOutcome[key] = msg
	return !seen || previous != msg
}

// FormatOutcome renders a non-pass outcome as a one-line human message.
func FormatOutcome(o Outcome) string {
	detail := strings.TrimSpace(o.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(o.Stdout)
	}
	tag := string(o.Hook.Scope) + "/" + string(o.Hook.Event)
	cmd := o.Hook.Command
	if cmd == "" && o.Hook.ContextFile != "" {
		cmd = "context:" + o.Hook.ContextFile
	}
	cmd = clipRunes(cmd, 60)
	trunc := ""
	if o.Truncated {
		trunc = " (output truncated)"
	}
	head := fmt.Sprintf("hook [%s] %s — %s%s", tag, cmd, o.Decision, trunc)
	if detail != "" {
		return head + ": " + detail
	}
	return head
}

func clipRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 1 {
		return ""
	}
	return string(r[:max]) + "…"
}
