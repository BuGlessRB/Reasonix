package control

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/event"
)

// Reading or operating another application is answered by a person in auto,
// once per application for the session, and by YOLO.
func TestComputerUseNeedsAPersonPerApplicationInAuto(t *testing.T) {
	approvals := make(chan event.Approval, 4)
	c := New(Options{Sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.ApprovalRequest {
			approvals <- e.Approval
		}
	})})
	c.SetToolApprovalMode(ToolApprovalAuto)
	approve := func(tool, app string) chan bool {
		done := make(chan bool, 1)
		go func() {
			allow, _, _ := gateApprover{c}.Approve(context.Background(), tool, app, nil)
			done <- allow
		}()
		return done
	}
	answer := func(session bool) event.Approval {
		t.Helper()
		select {
		case a := <-approvals:
			c.Approve(a.ID, true, session, false)
			return a
		case <-time.After(30 * time.Second):
			t.Fatal("computer use did not ask a person in auto")
		}
		return event.Approval{}
	}

	done := approve("computer_read", "com.apple.Notes")
	if a := answer(true); a.ReasonCode != computerUseApproval || a.Reason != explicitApprovalTexts[computerUseApproval] || a.Subject != "com.apple.Notes" {
		t.Fatalf("approval = %+v, want the application and why a person is needed", a)
	}
	if !<-done {
		t.Fatal("an approved read was refused")
	}
	if allow, _, err := (gateApprover{c}).Approve(context.Background(), "computer_act", "com.apple.Notes", nil); err != nil || !allow {
		t.Fatalf("operating the approved application = (%v, %v), want allow without asking", allow, err)
	}
	done = approve("computer_act", "com.apple.TextEdit")
	answer(false)
	if !<-done {
		t.Fatal("an approved call on another application was refused")
	}

	c.SetToolApprovalMode(ToolApprovalYolo)
	if allow, _, err := (gateApprover{c}).Approve(context.Background(), "computer_act", "com.apple.Mail", nil); err != nil || !allow {
		t.Fatalf("YOLO = (%v, %v), want allow", allow, err)
	}
	select {
	case extra := <-approvals:
		t.Fatalf("asked more than once per application, or under YOLO: %+v", extra)
	default:
	}
}
