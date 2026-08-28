// steer_inbox.go — the mid-turn guidance a running turn will accept.
package agent

import (
	"sync"

	"reasonix/internal/event"
	"reasonix/internal/provider"
)

type steerEntry struct {
	itemID string
	load   func() (string, error)
	// text is a fallback when load is nil (legacy Steer(string) path).
	text string
	host bool
}

// steerInbox is the guidance admitted while a Run is executing. The queue and
// the two flags share one lifetime — opened on the way into Run, closed on the
// way out — so they move as one state rather than as separate flags a caller
// could leave in a combination no Run ever produces.
type steerInbox struct {
	mu    sync.Mutex
	queue []steerEntry
	// drainedQueue is true when the queue emptied on the last take, which is
	// what tells a caller its guidance has been picked up.
	drainedQueue bool
	// running is true while a Run is executing. Intake is open only then.
	running bool
}

// open admits guidance for the Run that is starting.
func (s *steerInbox) open() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drainedQueue = false
	s.running = true
}

// admit queues guidance and reports whether intake was open. On false nothing
// was queued and the caller has to deliver it another way.
func (s *steerInbox) admit(e steerEntry) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return false
	}
	s.queue = append(s.queue, e)
	s.drainedQueue = false
	return true
}

// take removes the next entry. Loading its body is the caller's, so no inbox
// lock is held across the read.
func (s *steerInbox) take() (steerEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return steerEntry{}, false
	}
	e := s.queue[0]
	s.queue = s.queue[1:]
	s.drainedQueue = len(s.queue) == 0
	return e, true
}

// remove drops the entry for itemID before anything reads it, reporting
// whether it was still queued. This lock is the only place that can tell "not
// read yet" from "already on its way to the model" without racing take, which
// is what makes a cancel safe to act on rather than a guess.
func (s *steerInbox) remove(itemID string) bool {
	if itemID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.queue {
		if e.itemID != itemID {
			continue
		}
		s.queue = append(s.queue[:i], s.queue[i+1:]...)
		return true
	}
	return false
}

// DropSteer takes back guidance this turn accepted but has not read yet. It
// reports false once the run loop has taken the entry, which is the moment the
// text stops being the user's to withdraw.
func (a *Agent) DropSteer(itemID string) bool {
	return a.steer.remove(itemID)
}

// drained reports that the queue emptied on the last take.
func (s *steerInbox) drained() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.drainedQueue
}

func (s *steerInbox) pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// closeIfIdle closes intake only when nothing is waiting, which is what makes
// the check and the close one step: guidance accepted before it keeps the loop
// alive, and anything arriving after is refused.
func (s *steerInbox) closeIfIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) > 0 {
		return false
	}
	s.running = false
	return true
}

// close ends intake and hands back whatever was never consumed.
func (s *steerInbox) close() []steerEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := s.queue
	s.queue = nil
	if len(pending) > 0 {
		s.drainedQueue = true
	}
	s.running = false
	return pending
}

// SteerHostItem is SteerItem for a durable item the runtime authored. The
// attribution travels with the entry rather than with the call site, so every
// path that renders or persists it later says the same thing about who spoke.
func (a *Agent) SteerHostItem(itemID string, load func() (string, error)) bool {
	return a.queueSteer(steerEntry{itemID: itemID, load: load, host: true})
}

// RecordUnappliedHostSteer is RecordUnappliedSteer for runtime-authored
// guidance. Recording it as the user's would put words in their mouth in the
// one transcript a later turn reads back.
func (a *Agent) RecordUnappliedHostSteer(text string, itemID ...string) {
	a.recordUnappliedSteer(text, true, itemID...)
}

func (a *Agent) recordUnappliedSteer(text string, host bool, itemID ...string) {
	if a == nil || a.sess.conversation == nil {
		return
	}
	id := ""
	if len(itemID) > 0 {
		id = itemID[0]
	}
	a.sess.conversation.Add(provider.Message{
		Role:       provider.RoleTool,
		Content:    a.withTurnPreferences(midTurnSteerMessage(text, host)),
		ToolCallID: provider.LocalOnlyToolID,
		Name:       provider.LocalOnlyToolName,
		LocalOnly:  true,
	})
	a.svc.sink.Emit(event.Event{
		Kind:   event.Notice,
		Level:  event.LevelWarn,
		Code:   event.NoticeCodeUnappliedSteer,
		Text:   UnappliedSteerNotice(text),
		ItemID: id,
	})
}
