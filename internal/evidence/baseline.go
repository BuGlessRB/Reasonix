// The turn's mutation baseline: which change the host measures the remaining
// obligations from. Kept apart from the ledger's recording so the question
// "what still counts as changed" has one place to be answered.
package evidence

func (l *Ledger) LatestSuccessfulWriterIndex() (int, bool) {
	return l.LatestSuccessfulWriterIndexFunc(nil)
}

// LatestSuccessfulWriterIndexFunc is LatestSuccessfulWriterIndex with the
// caller deciding which writes still count. A scratch file written, used, and
// removed inside the turn left nothing to verify, and only the host can say so
// — it has to look at the disk — so the ledger takes the answer instead of
// guessing it. nil keeps every write.
func (l *Ledger) LatestSuccessfulWriterIndexFunc(keep func(Receipt) bool) (int, bool) {
	if l == nil {
		return 0, false
	}
	latest := -1

	l.mu.Lock()
	defer l.mu.Unlock()
	for i, r := range l.receipts {
		if r.Success && r.Write && (keep == nil || keep(r)) {
			latest = i
		}
	}
	return latest, latest >= 0
}

// LatestSuccessfulMutationIndex returns the most recent host-observed
// state-changing call. It includes known file writers, writer-capable delegated
// or external tools, and bash commands that are not demonstrably observational
// or verification-only.
func (l *Ledger) LatestSuccessfulMutationIndex() (int, bool) {
	if l == nil {
		return 0, false
	}
	latest := -1
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, r := range l.receipts {
		if r.Success && r.Mutation {
			latest = i
		}
	}
	return latest, latest >= 0
}

// LatestProvenMutationIndex is the baseline for what a change owes: the latest
// write the host could prove, or — when it proved none — the latest it could
// not classify. A check that merely resists classification must not become the
// change set, or every post-verification `gofmt -l` moves the goalposts past
// the verification that just ran.
func (l *Ledger) LatestProvenMutationIndex() (int, bool) {
	return l.LatestProvenMutationIndexFunc(nil)
}

// LatestProvenMutationIndexFunc is LatestProvenMutationIndex with the same
// caller veto LatestSuccessfulWriterIndexFunc takes, for the same reason.
// Unproven mutations are never vetoed: the host does not know what they
// touched, so it cannot know they left nothing behind.
func (l *Ledger) LatestProvenMutationIndexFunc(keep func(Receipt) bool) (int, bool) {
	if l == nil {
		return 0, false
	}
	proven, unproven := -1, -1
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, r := range l.receipts {
		if !r.Success || !r.Mutation {
			continue
		}
		if r.MutationEvidence == MutationProven {
			if keep == nil || keep(r) {
				proven = i
			}
			continue
		}
		unproven = i
	}
	if proven >= 0 {
		return proven, true
	}
	return unproven, unproven >= 0
}
