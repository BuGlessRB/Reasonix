package agent

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"reasonix/internal/event"
	"reasonix/internal/evidence"
	"reasonix/internal/permission"
	"reasonix/internal/provider"
	"reasonix/internal/tool"
)

// toolOutcome is one tool call's result. output is the first-visible bounded
// form the model sees; rawOutput is the full original when truncation applied
// (empty when identical so we avoid double storage). images ride outside text.
type toolOutcome struct {
	output       string
	rawOutput    string // full original when different from output
	images       []string
	blocked      bool
	errMsg       string
	bound        event.OutputBound
	truncMsg     string
	resolved     bool
	resolvedName string
	capabilityID string
	// resolvedProfile is the delegation behind a proxy call: the model only
	// ever sees use_capability, so without this a dispatched sub-agent reaches
	// the frontend anonymous.
	resolvedProfile            *event.Profile
	resolvedReadOnly, executed bool
	workspaceMutation          *event.WorkspaceMutation
	effective                  workspaceEffectiveCall
	// execution is local shell metadata (optional). Provider messages strip it
	// via ModelMessages; UI/event sinks surface it on ToolResult cards.
	execution *tool.ShellExecution
	// todoEcho marks a todo_write that wrote the list the host already held.
	todoEcho bool
}

// refusedShellExecution describes a shell call the host stopped before launch,
// so a refusal carries the metadata a failure would and stays visible to tool
// cards and trajectory digests. phase names the gate that stopped it.
func refusedShellExecution(t tool.Tool, args json.RawMessage, phase string) *tool.ShellExecution {
	ex := &tool.ShellExecution{Kind: "shell"}
	if bt, ok := t.(tool.DetailedExecutor); ok {
		if desc := bt.ExecutionDescriptor(args); desc != nil {
			ex = desc
		}
	}
	ex.State = tool.ShellStateNotRun
	ex.FailurePhase = phase
	ex.MutationRisk = tool.ShellMutationNotStarted
	ex.Verification = tool.ShellVerificationNotVerification
	return ex
}

// shellRefusal is refusedShellExecution for gates that run before the call is
// known to be a shell: only a tool that can describe an execution is one.
func shellRefusal(t tool.Tool, args json.RawMessage, phase string) *tool.ShellExecution {
	if _, ok := t.(tool.DetailedExecutor); !ok {
		return nil
	}
	return refusedShellExecution(t, args, phase)
}

// batchExecution is the result of one provider tool-call batch.
type batchExecution struct {
	results    []string
	outcomes   []toolOutcome
	images     [][]string
	executions []*tool.ShellExecution
}

// executeBatch dispatches one model turn's tool calls. ToolDispatch events are
// emitted up front in call order; contiguous known ReadOnly calls fan out
// across goroutines while unknown and writer calls run serially so write/read
// ordering stays provider-ordered. ToolResult events are emitted after the
// batch in call order. Images are aligned by index with results.
func (a *Agent) executeBatch(ctx context.Context, turn *turnRuntime, calls []provider.ToolCall) batchExecution {
	// The assistant message already stored this slice in Session. Keep execution
	// state separate so refreshing a dependent preview never mutates shared
	// session memory outside Session's lock.
	calls = append([]provider.ToolCall(nil), calls...)
	for _, c := range calls {
		a.emitEagerToolDispatch(ctx, c)
	}

	results := make([]string, len(calls))
	outcomes := make([]toolOutcome, len(calls))
	durations := make([]int64, len(calls))
	startedAt := make([]int64, len(calls))
	completedStepInBatch := false
	receiptMark := a.ledgerMark()
	// Full dispatches used the batch's initial file state. After a writer runs
	// (even a failed one — disk may have mutated), refresh dependent writer
	// previews. The first writer stays on the single-preview fast path.
	earlierWriterRan := false
	surfaceWriters := make([]bool, len(calls))
	run := func(i int) {
		t, _, ambiguous := a.svc.tools.ResolveCall(calls[i].Name)
		known := t != nil && len(ambiguous) == 0
		writer := known && !t.ReadOnly()
		surfaceWriters[i] = writer
		if earlierWriterRan && writer {
			if refreshed, changed := refreshCurrentFileDiff(ctx, t, calls[i]); changed {
				calls[i] = refreshed
				a.sess.conversation.UpdateToolCallPreview(refreshed)
				a.emitFullToolDispatch(ctx, refreshed, true)
			}
		}
		start := time.Now()
		startedAt[i] = start.UnixMilli()
		if calls[i].Name == "complete_step" && completedStepInBatch {
			output := "blocked: only one successful complete_step is allowed per tool-call round. Continue from the newly promoted in_progress todo in the next round instead of batching sign-offs."
			outcomes[i] = toolOutcome{output: output, blocked: true, errMsg: "blocked: complete_step sign-offs must be serial"}
			if a.task.ledger != nil {
				a.task.ledger.Record(evidence.ReceiptFromToolCall(calls[i].Name, json.RawMessage(calls[i].Arguments), false, evidence.ToolFacts{ReadOnly: true}))
			}
			durations[i] = time.Since(start).Milliseconds()
			results[i] = output
			return
		}
		outcomes[i] = a.executeOne(ctx, turn, calls[i])
		recordWorkspaceMutation(a.svc.sink, outcomes[i].workspaceMutation)
		if outcomes[i].executed {
			surfaceWriters[i] = outcomes[i].workspaceMutation != nil
		}
		if outcomes[i].resolved {
			readOnly := outcomes[i].resolvedReadOnly
			calls[i].ResolvedName = outcomes[i].resolvedName
			calls[i].CapabilityID = outcomes[i].capabilityID
			calls[i].ResolvedReadOnly = &readOnly
			surfaceWriters[i] = !readOnly
		}
		if calls[i].Name == "complete_step" && outcomes[i].errMsg == "" {
			completedStepInBatch = true
		}
		durations[i] = time.Since(start).Milliseconds()
		results[i] = outcomes[i].output
	}
	finalize := func(i int) {
		if calls[i].ResolvedReadOnly != nil {
			a.sess.conversation.UpdateToolCallResolution(calls[i])
			a.emitResolvedToolDispatch(calls[i], outcomes[i].resolvedProfile)
		}
		if surfaceWriters[i] || (outcomes[i].resolved && !outcomes[i].resolvedReadOnly) {
			earlierWriterRan = true
		}
	}
	cancelled := false
	markCancelled := func(start int) {
		errMsg := context.Canceled.Error()
		if err := ctx.Err(); err != nil {
			errMsg = err.Error()
		}
		output := "cancelled: context cancelled before execution"
		for j := start; j < len(calls); j++ {
			results[j] = output
			outcomes[j] = toolOutcome{output: output, errMsg: errMsg}
		}
		cancelled = true
	}

	// Deterministic dependency barrier: after a mutating call fails or is
	// blocked, later mutations/verifications in the batch are skipped; read-only
	// diagnosis still runs. executeOne re-checks after proxy resolution.
	mutationBatchStop := false
	a.mutationDependencyBarrier.Store(false)
	markDependencySkipped := func(start int) {
		a.mutationDependencyBarrier.Store(true)
		for j := start; j < len(calls); j++ {
			if results[j] != "" {
				continue
			}
			// Pre-classify when statically certain. Proxies and ambiguous
			// targets fall through to run() so executeOne can resolve the real
			// target and re-apply the barrier before Commit/Execute.
			if !batchCallStaticallySkippable(a, calls[j]) {
				continue
			}
			isVerification := calls[j].Name == "bash" && evidence.IsDeliveryVerificationCommand(bashCommandFromArgs(json.RawMessage(calls[j].Arguments)))
			msg := "blocked: skipped because an earlier modification in this tool batch failed or was blocked. " +
				"Fix or re-run the failed change first; verification was not executed."
			var ex *tool.ShellExecution
			if calls[j].Name == "bash" {
				resolved, _, _ := a.svc.tools.ResolveCall(calls[j].Name)
				ex = refusedShellExecution(resolved, json.RawMessage(calls[j].Arguments), tool.ShellPhaseDependency)
				if isVerification {
					ex.Verification = tool.ShellVerificationNotRun
				}
			}
			results[j] = msg
			outcomes[j] = toolOutcome{
				output:    msg,
				blocked:   true,
				errMsg:    firstLine(msg),
				execution: ex,
			}
			durations[j] = 0
		}
		mutationBatchStop = true
	}

	for _, batch := range partitionToolCalls(ctx, a.svc.tools, calls) {
		if ctx.Err() != nil {
			markCancelled(batch.start)
			break
		}
		if batch.parallel && batch.end-batch.start > 1 {
			// Parallel segments are read-only by construction; no mutation barrier.
			ranUntil := runParallel(ctx, batch.start, batch.end, run)
			for i := batch.start; i < ranUntil; i++ {
				finalize(i)
			}
			// After parallel execution completes, check if context was cancelled.
			// The individual tool executions should have detected ctx.Done(), but
			// we verify here to ensure we don't continue to subsequent batches.
			if ctx.Err() != nil {
				markCancelled(ranUntil)
				break
			}
			continue
		}
		for i := batch.start; i < batch.end; i++ {
			// Before executing the next tool, check if context was cancelled.
			// This prevents starting new tools when a previous tool's execution
			// triggered cancellation.
			if ctx.Err() != nil {
				markCancelled(i)
				break
			}
			if mutationBatchStop {
				// Fill dependency skips for remaining mutating/verify calls, then
				// allow any residual read-only diagnosis to run individually.
				if results[i] != "" {
					continue
				}
				t, _, ambiguous := a.svc.tools.ResolveCall(calls[i].Name)
				known := t != nil && len(ambiguous) == 0
				readOnly := known && t.ReadOnly()
				if calls[i].Name == "bash" && permission.BashCommandIsReadOnly(json.RawMessage(calls[i].Arguments)) {
					readOnly = true
				}
				isVerification := calls[i].Name == "bash" && evidence.IsDeliveryVerificationCommand(bashCommandFromArgs(json.RawMessage(calls[i].Arguments)))
				mutates := evidence.ToolCallMutates(calls[i].Name, json.RawMessage(calls[i].Arguments), readOnly)
				if mutates || isVerification {
					markDependencySkipped(i)
					// markDependencySkipped fills this index; move on.
					if results[i] != "" {
						continue
					}
				}
			}
			if results[i] != "" {
				// Pre-filled dependency skip.
				finalize(i)
				continue
			}
			run(i)
			finalize(i)
			// Mutation/verification failure barrier for the rest of this batch.
			if batchCallIsMutatingFailure(a, calls[i], outcomes[i]) {
				mutationBatchStop = true
				markDependencySkipped(i + 1)
			}
			// After each tool execution, also check if the context was cancelled.
			// If so, stop executing remaining tools and return immediately so
			// the agent loop can detect the cancellation and exit.
			if ctx.Err() != nil {
				markCancelled(i + 1)
				break
			}
		}
		if cancelled {
			break
		}
	}

	for i, c := range calls {
		o := outcomes[i]
		t, _, ambiguous := a.svc.tools.ResolveCall(c.Name)
		ok := t != nil && len(ambiguous) == 0
		readOnly := ok && t.ReadOnly()
		if c.ResolvedReadOnly != nil {
			readOnly = *c.ResolvedReadOnly
		}
		tr := event.Tool{
			ID:           c.ID,
			Name:         c.Name,
			Args:         c.Arguments,
			ResolvedName: c.ResolvedName,
			CapabilityID: c.CapabilityID,
			Output:       o.output,
			Err:          o.errMsg,
			ReadOnly:     readOnly,
			Bound:        o.bound,
			DurationMs:   durations[i],
			Execution:    toEventShellExecution(o.execution, durations[i]),
		}
		if startedAt[i] > 0 {
			tr.StartedAt = startedAt[i]
			tr.EndedAt = startedAt[i] + durations[i]
			if mutation := o.workspaceMutation; mutation != nil {
				tr.WorkspaceMutation = true
				tr.WorkspacePaths = append([]string(nil), mutation.Paths...)
				tr.WorkspaceAllPaths = mutation.AllPaths
			}
		}
		a.emitToolCard(ctx, c, tr, o.todoEcho)
		if o.truncMsg != "" {
			a.svc.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Text: o.truncMsg})
		}
	}
	a.observeOutcomeShadow(cancelled, receiptMark)
	images := make([][]string, len(calls))
	executions := make([]*tool.ShellExecution, len(calls))
	for i := range outcomes {
		images[i] = outcomes[i].images
		executions[i] = outcomes[i].execution
	}
	return batchExecution{
		results:    results,
		outcomes:   outcomes,
		images:     images,
		executions: executions,
	}
}

// batchCallIsMutatingFailure reports whether a finished call was a mutation
// (file write / non-readonly bash mutation) that failed or was blocked, so later
// mutations and verifications in the same batch must not run.
func batchCallIsMutatingFailure(a *Agent, call provider.ToolCall, o toolOutcome) bool {
	if o.errMsg == "" && !o.blocked {
		return false
	}
	readOnly := false
	toolName := call.Name
	toolArgs := json.RawMessage(call.Arguments)
	t, _, ambiguous := a.svc.tools.ResolveCall(call.Name)
	known := t != nil && len(ambiguous) == 0
	if known {
		readOnly = t.ReadOnly()
	}
	if call.ResolvedReadOnly != nil {
		readOnly = *call.ResolvedReadOnly
	}
	if o.resolved {
		readOnly = o.resolvedReadOnly
	}
	if o.effective.name != "" {
		toolName = o.effective.name
		toolArgs = o.effective.args
		readOnly = o.effective.readOnly
	}
	if toolName == "bash" && permission.BashCommandIsReadOnly(toolArgs) {
		readOnly = true
	}
	// Verification failures do not open the dependency barrier by themselves —
	// only a failed modification does.
	if toolName == "bash" && evidence.IsDeliveryVerificationCommand(bashCommandFromArgs(toolArgs)) {
		return false
	}
	// Resolved writers (including MCP targets behind use_capability) count even
	// when the provider-visible proxy advertised ReadOnly.
	if o.resolved && !o.resolvedReadOnly {
		return true
	}
	if evidence.ToolCallMutates(toolName, toolArgs, readOnly) {
		return true
	}
	// Fail closed only for a target the host could not classify: a blanket
	// !readOnly fallback would re-admit the meta tools ToolCallMutates exempts,
	// letting a failed todo_write block every real edit left in the batch.
	return !known
}

// batchCallStaticallySkippable reports whether a remaining call can be marked
// not_run/dependency without resolving a proxy. Proxies and unknown tools
// return false so executeOne can resolve the real target first.
func batchCallStaticallySkippable(a *Agent, call provider.ToolCall) bool {
	t, _, ambiguous := a.svc.tools.ResolveCall(call.Name)
	if t == nil || len(ambiguous) > 0 {
		// Unknown / ambiguous: fail closed via executeOne path.
		return false
	}
	// A proxy may resolve against a live capability whose result can change
	// between calls, so never resolve here just to pre-fill a skip: executeOne
	// resolves exactly once and classifies the real target before Commit.
	if _, ok := t.(tool.CallResolver); ok {
		return false
	}
	readOnly := t.ReadOnly()
	if call.Name == "bash" && permission.BashCommandIsReadOnly(json.RawMessage(call.Arguments)) {
		readOnly = true
	}
	isVerification := call.Name == "bash" && evidence.IsDeliveryVerificationCommand(bashCommandFromArgs(json.RawMessage(call.Arguments)))
	if isVerification {
		return true
	}
	return !readOnly || evidence.ToolCallMutates(call.Name, json.RawMessage(call.Arguments), readOnly)
}

type toolCallBatch struct {
	start    int
	end      int
	parallel bool
}

// partitionToolCalls keeps provider order while letting contiguous read-only
// tools run together. A writer, an unresolvable name, and a tool that declares
// it needs its own place each get a single-call serial batch.
func partitionToolCalls(ctx context.Context, r *tool.Registry, calls []provider.ToolCall) []toolCallBatch {
	var batches []toolCallBatch
	for i := 0; i < len(calls); {
		if parallelisable(ctx, r, calls[i]) {
			start := i
			i++
			for i < len(calls) && parallelisable(ctx, r, calls[i]) {
				i++
			}
			batches = append(batches, toolCallBatch{start: start, end: i, parallel: true})
			continue
		}
		batches = append(batches, toolCallBatch{start: i, end: i + 1})
		i++
	}
	return batches
}

func parallelisable(ctx context.Context, r *tool.Registry, call provider.ToolCall) bool {
	t, canonical, ambiguous := r.ResolveCall(call.Name)
	if t == nil || len(ambiguous) > 0 {
		return false
	}
	args := json.RawMessage(call.Arguments)
	// ReadOnly says a call needs no approval; it does not say the call may share
	// a batch. A tool that advances host state or reads what an earlier call in
	// this reply started answers that second question itself.
	if tool.RunsSequentially(ctx, t, args) {
		return false
	}
	if t.ReadOnly() {
		return true
	}
	// Bash is writer-capable in its schema, so it never joined a parallel run
	// even when its arguments read as read-only — the same fact permission,
	// mutation accounting, and evidence already act on.
	return canonical == "bash" && permission.BashCommandIsReadOnly(args)
}

func runParallel(ctx context.Context, start, end int, run func(int)) int {
	const maxParallel = 8
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	ranUntil := start
launch:
	for i := start; i < end; i++ {
		if ctx.Err() != nil {
			break
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break launch
		}
		if ctx.Err() != nil {
			<-sem
			break
		}

		wg.Add(1)
		ranUntil = i + 1
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			run(i)
		}()
	}
	wg.Wait()
	return ranUntil
}
