package boot

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
	"reasonix/internal/runtime/agent"
	"reasonix/internal/runtime/bestof"
	"reasonix/internal/session/control"
)

// candidateApproval is the session's approval posture, read when a best_of_n
// call runs. Approving that one call is what lets its candidates run
// unattended, so they get auto — deny and ask rules still hold — unless the
// session refuses everything, which they inherit.
type candidateApproval struct {
	ctrl atomic.Pointer[control.Controller]
}

func (c *candidateApproval) bind(ctrl *control.Controller) {
	if c != nil {
		c.ctrl.Store(ctrl)
	}
}

func (c *candidateApproval) mode() string {
	if ctrl := c.ctrl.Load(); ctrl != nil && ctrl.ToolApprovalMode() == control.ToolApprovalDontAsk {
		return control.ToolApprovalDontAsk
	}
	return control.ToolApprovalAuto
}

// addBestOf offers best_of_n when [agent] best_of_n is on. A candidate's own
// kernel never offers it, so attempts cannot fan out again.
func (b *builder) addBestOf() {
	if !b.cfg.Agent.BestOfN || b.opts.BestOfCandidate {
		return
	}
	approval := &candidateApproval{}
	b.tools.candidates = approval
	judge := bestof.JudgeSpec{Provider: b.execProv, ModelRef: b.model.ref, Sink: b.sink}
	if b.model.entry != nil {
		judge.Pricing = b.model.entry.Price
	}
	if ref := strings.TrimSpace(b.cfg.Agent.AdvisorModel); ref != "" {
		if entry, ok := b.cfg.ResolveModel(ref); ok {
			if prov, err := NewProviderWithProxy(entry, b.proxy); err == nil {
				judge = bestof.JudgeSpec{Provider: prov, ModelRef: modelRefFromEntry(entry), Pricing: entry.Price, Sink: b.sink}
			}
		}
	}
	cfg := b.cfg
	b.tools.reg.Add(bestof.New(bestof.Spec{
		WorkspaceRoot: b.root,
		ManagedRoot:   filepath.Join(config.DeliveryWorktreeDir(), "candidates"),
		Runner:        candidateRunner(b.opts, b.sink, approval),
		KnownModel:    func(ref string) bool { _, ok := cfg.ResolveModel(ref); return ok },
		Judge:         judge,
	}))
}

// candidateRunner builds one throwaway kernel per attempt, rooted at the
// attempt's worktree, and runs the prompt to its end. Its transcript lives in a
// temp dir removed with it; only its usage reaches the parent session.
func candidateRunner(parent Options, sink event.Sink, approval *candidateApproval) bestof.Runner {
	return func(ctx context.Context, run bestof.Run) (bestof.Outcome, error) {
		sessDir, err := os.MkdirTemp("", "reasonix-candidate-")
		if err != nil {
			return bestof.Outcome{}, err
		}
		defer os.RemoveAll(sessDir)
		ctrl, err := Build(ctx, Options{
			WorkspaceRoot:        run.WorkspaceRoot,
			Home:                 parent.Home,
			Model:                run.Model,
			AgentPreset:          parent.AgentPreset,
			ProviderResolver:     parent.ProviderResolver,
			Sink:                 candidateUsage{sink},
			Stderr:               io.Discard,
			SessionDir:           sessDir,
			HeadlessApprovalMode: approval.mode(),
			ApprovalTimeout:      time.Second,
			GoalTurnsUnreachable: true,
			BestOfCandidate:      true,
		})
		if err != nil {
			return bestof.Outcome{}, err
		}
		defer ctrl.Close()
		out := bestof.Outcome{}
		if err := ctrl.Run(ctx, run.Prompt); err != nil {
			var unready *agent.FinalReadinessError
			if !errors.As(err, &unready) {
				return bestof.Outcome{}, err
			}
			out.Unverified = unready.Reason
		}
		out.Answer = finalAnswer(ctrl)
		return out, nil
	}
}

// finalAnswer is the attempt's last visible message; empty when it ended
// without one, which the judge reads as an attempt with nothing to say.
func finalAnswer(ctrl *control.Controller) string {
	for _, m := range slices.Backward(ctrl.History()) {
		if m.Role == provider.RoleAssistant && !m.LocalOnly && strings.TrimSpace(m.Content) != "" {
			return m.Content
		}
	}
	return ""
}

// candidateUsage forwards a candidate's billable usage to the parent session
// under the best-of source and drops everything else it emits.
type candidateUsage struct{ parent event.Sink }

func (s candidateUsage) Emit(e event.Event) {
	if e.Kind != event.Usage || s.parent == nil {
		return
	}
	e.UsageSource, e.Source = event.UsageSourceBestOf, event.UsageSourceBestOf
	s.parent.Emit(e)
}
