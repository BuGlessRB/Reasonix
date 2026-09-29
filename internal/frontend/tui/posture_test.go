package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/base/i18n"
)

type posture struct {
	mode string
	plan bool
}

// settle runs cmd against the kernel and then answers /status the way the
// kernel would have, since the recording kernel reports nothing back.
func settle(m *model, cmd tea.Cmd) posture {
	p := posture{m.status.ToolApprovalMode, m.status.Plan}
	run(m, cmd)
	m.status.ToolApprovalMode, m.status.Plan = p.mode, p.plan
	return p
}

func TestShiftTabCyclesInTheOrder1xUsed(t *testing.T) {
	m, k := testModel(t)
	m.opts.YoloConfirmed = true
	m.status.ToolApprovalMode = "readOnly"
	want := []posture{{"ask", false}, {"auto", false}, {"yolo", false}, {"ask", true}, {"readOnly", false}, {"ask", false}}
	for i, w := range want {
		if got := settle(m, m.cycleMode()); got != w {
			t.Fatalf("step %d: Shift+Tab gave %+v, want %+v", i, got, w)
		}
	}
	calls := strings.Join(k.seen(), "\n")
	for _, c := range []string{`/tool-approval-mode {"mode":"readOnly"}`, `/plan {"on":true}`, `/plan {"on":false}`, `/tool-approval-mode {"mode":"yolo"}`} {
		if !strings.Contains(calls, c) {
			t.Errorf("the kernel never saw %s:\n%s", c, calls)
		}
	}
}

func TestShiftTabSkipsYoloUntilItWasConfirmed(t *testing.T) {
	m, _ := testModel(t)
	m.status.ToolApprovalMode = "auto"
	if got := settle(m, m.cycleMode()); got != (posture{"auto", true}) {
		t.Fatalf("an unconfirmed YOLO is not a stop on the cycle: got %+v", got)
	}
}

func TestCtrlYAsksOnceThenReturnsToThePostureItLeft(t *testing.T) {
	m, k := testModel(t)
	recorded := 0
	m.opts.ConfirmYolo = func() error { recorded++; return nil }
	m.status.ToolApprovalMode = "auto"

	if got := settle(m, m.toggleYolo()); got.mode != "auto" {
		t.Fatalf("the first Ctrl+Y only explains; mode moved to %q", got.mode)
	}
	if !strings.Contains(noticeText(m), i18n.M.YoloConfirmHint) {
		t.Fatal("the first Ctrl+Y must say what YOLO skips")
	}
	for _, c := range k.seen() {
		if strings.Contains(c, "yolo") {
			t.Fatalf("an unconfirmed Ctrl+Y reached the kernel: %s", c)
		}
	}
	if got := settle(m, m.toggleYolo()); got.mode != "yolo" || recorded != 1 || !m.opts.YoloConfirmed {
		t.Fatalf("the second Ctrl+Y confirms: mode %q, recorded %d", got.mode, recorded)
	}
	if got := settle(m, m.toggleYolo()); got.mode != "auto" {
		t.Fatalf("leaving YOLO goes back to auto, got %q", got.mode)
	}
	m.status.ToolApprovalMode = "readOnly"
	settle(m, m.toggleYolo())
	if recorded != 1 {
		t.Fatalf("a confirmed YOLO is not asked about again (recorded %d)", recorded)
	}
	if got := settle(m, m.toggleYolo()); got.mode != "readOnly" {
		t.Fatalf("leaving YOLO goes back to read only, got %q", got.mode)
	}
}

func TestCtrlYStillEnablesWhenTheRecordFails(t *testing.T) {
	m, _ := testModel(t)
	m.opts.ConfirmYolo = func() error { return errors.New("disk full") }
	m.status.ToolApprovalMode = "ask"
	settle(m, m.toggleYolo())
	if got := settle(m, m.toggleYolo()); got.mode != "yolo" {
		t.Fatalf("the person confirmed; a failed record must not undo that, got %q", got.mode)
	}
	if !strings.Contains(noticeText(m), "disk full") {
		t.Fatal("a failed record is reported")
	}
	if got := settle(m, m.toggleYolo()); got.mode != "ask" {
		t.Fatalf("leaving YOLO goes back to ask, got %q", got.mode)
	}
}

func TestFooterNamesReadOnlyAndDontAskApart(t *testing.T) {
	m, _ := testModel(t)
	m.status.ToolApprovalMode = "readOnly"
	if tag := m.modeTag(); !strings.Contains(tag, "Read only") {
		t.Fatalf("read-only footer tag = %q", tag)
	}
	m.status.ToolApprovalMode = "dontAsk"
	if tag := m.modeTag(); strings.Contains(tag, "Read only") {
		t.Fatalf("dontAsk still reads as read only: %q", tag)
	}
}

func noticeText(m *model) string {
	var b strings.Builder
	for _, it := range m.tr.Items {
		b.WriteString(it.Text)
		b.WriteString("\n")
	}
	return b.String()
}
