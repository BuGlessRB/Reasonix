package evidence

// The ledger's per-round views: what a round contributed is scored from these
// slices by OutcomeTracker, the turn's single round scorer.

// ReceiptsSince returns a copy of the receipts recorded at or after index.
func (l *Ledger) ReceiptsSince(index int) []Receipt {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if index < 0 {
		index = 0
	}
	if index >= len(l.receipts) {
		return nil
	}
	return append([]Receipt(nil), l.receipts[index:]...)
}

// Receipts returns a copy of every receipt recorded this turn, in order —
// the replay feed for shadow observers that must not share ledger memory.
func (l *Ledger) Receipts() []Receipt {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Receipt(nil), l.receipts...)
}
