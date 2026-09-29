package tui

import (
	"fmt"

	"reasonix/internal/contract/eventwire"
)

// foldProgressWatch says a stall once, when it starts. The kernel reports it
// every round it lasts; repeating that would bury the transcript it is about,
// and a report that it cleared needs no row of its own.
func (t *Transcript) foldProgressWatch(w *eventwire.ProgressWatch) {
	if w == nil || !w.Stalled {
		t.stallSaid = false
		return
	}
	if t.stallSaid {
		return
	}
	t.stallSaid = true
	t.foldNotice(eventwire.Event{Kind: "notice", Level: "warn", Code: "progress_watch", Text: stallText(w)})
}

func stallText(w *eventwire.ProgressWatch) string {
	switch w.Cause {
	case "tokens":
		return fmt.Sprintf("About %d context windows of input (%d tokens) since the run last did anything observable. Whether to keep going is your call.",
			w.TokenMultiple, w.PromptTokens)
	case "perseveration":
		return "The model is repeating the same text. Whether to keep going is your call."
	default:
		return fmt.Sprintf("No observable progress for %d tool rounds: no file changed, no check or task step moved, nothing new was read. Whether to keep going is your call.",
			w.IdleRounds)
	}
}
