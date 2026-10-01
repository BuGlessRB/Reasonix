package boot

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/contract/event"
	"reasonix/internal/runtime/agent/testutil"
	"reasonix/internal/session/control"
	"reasonix/internal/state/sessioninbox"
	"reasonix/internal/state/sessionstore"
)

// TestEffectUnappliedQueuedSteerCarriesATypedBlockCode: a queued item accepted as
// guidance into a turn that the user then ends by skipping its question is
// held as uncertain, and the snapshot a frontend reads names why with a stable
// code rather than only an English sentence.
func TestEffectUnappliedQueuedSteerCarriesATypedBlockCode(t *testing.T) {
	root := observeProject(t)
	prov := testutil.NewMock("m",
		call("a1", "ask", `{"questions":[{"header":"Lib","question":"Which one?","options":[{"label":"A"},{"label":"B"}]}]}`),
		testutil.Turn{Text: "never reached"},
	)
	setBootTokenProfileTestProvider(t, prov)

	asked := make(chan event.Ask, 1)
	ended := make(chan struct{}, 4)
	sink := event.FuncSink(func(e event.Event) {
		switch e.Kind {
		case event.AskRequest:
			asked <- e.Ask
		case event.TurnDone:
			ended <- struct{}{}
		}
	})
	c, err := Build(context.Background(), Options{Sink: sink, WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(c.Close)
	c.SetSessionPath(sessionstore.NewSessionPath(c.SessionDir(), c.Label()))
	c.EnableInteractiveApproval()

	c.Send("pick one")
	var ask event.Ask
	select {
	case ask = <-asked:
	case <-time.After(30 * time.Second):
		t.Fatal("the question never opened")
	}
	if _, err := c.TryEnqueueAndSteer(control.InboxRequest{Display: "queued", Raw: "queued", Submit: "queued"}); err != nil {
		t.Fatalf("queue the guidance: %v", err)
	}
	c.AnswerQuestion(ask.ID, []event.AskAnswer{{QuestionID: ask.Questions[0].ID}})
	select {
	case <-ended:
	case <-time.After(30 * time.Second):
		t.Fatal("the turn never ended")
	}

	snap := c.InboxSnapshot()
	if len(snap.Items) != 1 || snap.Items[0].State != sessioninbox.StateUncertain {
		t.Fatalf("want the one queued item held as uncertain, got %+v", snap.Items)
	}
	if snap.Items[0].BlockCode != sessioninbox.BlockSteerUnapplied {
		t.Fatalf("BlockCode = %q, want %q", snap.Items[0].BlockCode, sessioninbox.BlockSteerUnapplied)
	}
}
