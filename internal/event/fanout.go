package event

import "reasonix/internal/evidence"

// FanOut dispatches each event to every registered sink in order.
// A nil sink in the list is silently skipped. Use it when you want one
// event stream to reach multiple consumers — e.g. the desktop tab UI and
// a bot-channel notifier.
type FanOut struct {
	sinks []Sink
}

// NewFanOut returns a FanOut that delivers every Emit call to every sink
// in the given order. A zero-length list is valid (no-op).
func NewFanOut(sinks ...Sink) *FanOut {
	return &FanOut{sinks: sinks}
}

// Emit forwards e to every registered sink. Nil sinks are skipped.
func (f *FanOut) Emit(e Event) {
	for _, s := range f.sinks {
		if s == nil {
			continue
		}
		s.Emit(e)
	}
}

// Len returns the number of registered sinks.
func (f *FanOut) Len() int { return len(f.sinks) }

// Optional capabilities reach every branch: a fan-out that forwarded only Emit
// would strand each audit channel at the split, and each helper already skips
// branches that do not opt in.

func (f *FanOut) RecordDelegationAudit(a evidence.DelegationAudit) {
	for _, s := range f.sinks {
		RecordDelegationAudit(s, a)
	}
}

func (f *FanOut) RecordReadinessAudit(a evidence.ReadinessAudit) {
	for _, s := range f.sinks {
		RecordReadinessAudit(s, a)
	}
}

func (f *FanOut) RecordProjectCheckProbe(p ProjectCheckProbe) {
	for _, s := range f.sinks {
		RecordProjectCheckProbe(s, p)
	}
}

func (f *FanOut) RecordSubagentHandoff(a SubagentHandoffAudit) {
	for _, s := range f.sinks {
		RecordSubagentHandoff(s, a)
	}
}

func (f *FanOut) RecordVerificationContractDrift(d VerificationContractDrift) {
	for _, s := range f.sinks {
		RecordVerificationContractDrift(s, d)
	}
}

func (f *FanOut) RecordTurnCompletion() {
	for _, s := range f.sinks {
		RecordTurnCompletion(s)
	}
}

func (f *FanOut) RecordProtocolRecovery(a ProtocolRecoveryAudit) {
	for _, s := range f.sinks {
		RecordProtocolRecovery(s, a)
	}
}

func (f *FanOut) RecordContractShadow(a ContractShadowAudit) {
	for _, s := range f.sinks {
		RecordContractShadow(s, a)
	}
}

func (f *FanOut) RecordCompletionReport(a CompletionReportAudit) {
	for _, s := range f.sinks {
		RecordCompletionReport(s, a)
	}
}

func (f *FanOut) RecordMemoryRecall(a MemoryRecallAudit) {
	for _, s := range f.sinks {
		RecordMemoryRecall(s, a)
	}
}

func (f *FanOut) RecordOutcomeProgress(sample evidence.OutcomeSample) {
	for _, s := range f.sinks {
		RecordOutcomeProgress(s, sample)
	}
}

func (f *FanOut) RecordWorkspaceMutation(m WorkspaceMutation) {
	for _, s := range f.sinks {
		RecordWorkspaceMutation(s, m)
	}
}

func (f *FanOut) RecordRunBudget(sample RunBudgetSample) {
	for _, s := range f.sinks {
		RecordRunBudget(s, sample)
	}
}
