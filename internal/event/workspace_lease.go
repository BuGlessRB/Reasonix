package event

// WorkspaceLease is one closed account of the workspace write lease. It decides
// nothing: no gate reads it and no prompt byte moves with it. Waits under the
// notice grace are counted here and nowhere else, and IdleMs is the stretch
// held after the last write asked for it.
type WorkspaceLease struct {
	Contended int   `json:"contended"`
	Reported  int   `json:"reported,omitempty"`
	WaitedMs  int64 `json:"waitedMs,omitempty"`
	HeldMs    int64 `json:"heldMs"`
	IdleMs    int64 `json:"idleMs"`
}
