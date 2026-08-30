package agent

// Re-projection of the canonical task list's identities. The list itself never
// rides in the prompt, so a fold can take the step ids out of the model's view
// while the host still holds them — and complete_step goes on asking for one.

import (
	"fmt"
	"slices"
	"strings"

	"reasonix/internal/evidence"
	"reasonix/internal/provider"
)

// noteTodoIdentityShown marks the canonical step ids as readable by the model.
func (a *Agent) noteTodoIdentityShown() {
	a.sess.todoMu.Lock()
	a.sess.todoIdentityShown = true
	a.sess.todoMu.Unlock()
}

// noteTodoIdentityLost marks them unreadable after a provider-visible rewrite.
func (a *Agent) noteTodoIdentityLost() {
	a.sess.todoMu.Lock()
	a.sess.todoIdentityShown = false
	a.sess.todoMu.Unlock()
}

// todoIdentityNote renders the ids a sign-off must cite. It states whose list
// this is: an unattributed task list reads as something the model itself sent.
func todoIdentityNote(todos []evidence.TodoItem) string {
	var b strings.Builder
	b.WriteString("Host task state. This list is the host's, not a message you sent; cite these step ids in complete_step:")
	for i, t := range todos {
		fmt.Fprintf(&b, "\n  - %s (%s)", evidence.TodoCitation(t.StepID, i+1, t.Content), t.Status)
	}
	return b.String()
}

// todoIdentityProjection is the turn-tail note owed when the host holds step ids
// the conversation no longer shows. It re-projects the identity, not the list:
// the canonical state stays out of the cache-stable prefix, and a model that
// can only see ordinals cites ordinals — which go stale on the next insertion.
func (a *Agent) todoIdentityProjection() string {
	todos := a.CanonicalTodoState()
	a.sess.todoMu.Lock()
	shown := a.sess.todoIdentityShown
	a.sess.todoMu.Unlock()
	if shown || len(evidence.TodoStepIDs(todos)) == 0 {
		return ""
	}
	a.noteTodoIdentityShown()
	return todoIdentityNote(todos)
}

// withTodoIdentityProjection carries the host's step ids into a fold's own
// output, because the fold is what removes the model's last copy of them: the
// request it feeds is frozen before the next round exists, so a sign-off sent
// in that round could only cite an ordinal. No turn preferences ride along —
// a projection is persisted and reused.
func (a *Agent) withTodoIdentityProjection(projected []provider.Message) []provider.Message {
	todos := a.CanonicalTodoState()
	ids := evidence.TodoStepIDs(todos)
	if len(ids) == 0 || todoIdentitiesVisible(projected, ids) {
		return projected
	}
	return append(projected, provider.Message{Role: provider.RoleUser, Content: todoIdentityNote(todos)})
}

// todoIdentitiesVisible reports whether every id can still be read in the view
// the model is about to be sent. Bounded by the projection, which is the part
// compaction just made small.
func todoIdentitiesVisible(msgs []provider.Message, ids []string) bool {
	for _, id := range ids {
		if !messagesMentionID(msgs, id) {
			return false
		}
	}
	return true
}

func messagesMentionID(msgs []provider.Message, id string) bool {
	for _, msg := range slices.Backward(msgs) {
		if strings.Contains(msg.Content, id) {
			return true
		}
		for _, call := range msg.ToolCalls {
			if strings.Contains(call.Arguments, id) {
				return true
			}
		}
	}
	return false
}
