package eventwire

import "reasonix/internal/event"

// WorkspaceLease is the JSON form of event.WorkspaceLease.
type WorkspaceLease struct {
	Contended int   `json:"contended"`
	Reported  int   `json:"reported,omitempty"`
	WaitedMs  int64 `json:"waitedMs,omitempty"`
	HeldMs    int64 `json:"heldMs"`
	IdleMs    int64 `json:"idleMs"`
}

func toWireWorkspaceLease(l *event.WorkspaceLease) *WorkspaceLease {
	if l == nil {
		return nil
	}
	return &WorkspaceLease{
		Contended: l.Contended, Reported: l.Reported,
		WaitedMs: l.WaitedMs, HeldMs: l.HeldMs, IdleMs: l.IdleMs,
	}
}

// wireLevel names a notice's severity. Only the warning tier is distinguished:
// everything else a frontend draws as information.
func wireLevel(level event.Level) string {
	if level == event.LevelWarn {
		return "warn"
	}
	return "info"
}
