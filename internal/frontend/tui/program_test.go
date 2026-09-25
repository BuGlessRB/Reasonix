package tui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/contract/eventwire"
)

// recordingKernel answers every route with success and records what the TUI
// asked of it.
type recordingKernel struct {
	mu    sync.Mutex
	calls []string
}

func (k *recordingKernel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	k.mu.Lock()
	k.calls = append(k.calls, r.Method+" "+r.URL.Path+" "+strings.TrimSpace(string(body)))
	k.mu.Unlock()
	switch r.URL.Path {
	case "/inbox/items":
		_ = json.NewEncoder(w).Encode(map[string]string{"itemId": "q-7"})
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (k *recordingKernel) seen() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.calls...)
}

func testModel(t *testing.T) (*model, *recordingKernel) {
	t.Helper()
	k := &recordingKernel{}
	srv := httptest.NewServer(k)
	t.Cleanup(srv.Close)
	m := newModel(context.Background(), Options{Client: &Client{HTTP: srv.Client(), Base: srv.URL}})
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return m, k
}

// run executes a command tree the way the program would, feeding every
// message it produces back into the model, except prints and ticks.
func run(m *model, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			run(m, c)
		}
	case nil, statusTickMsg:
	default:
		if _, isSeq := msg.(tea.Cmd); isSeq {
			return
		}
		_, next := m.Update(msg)
		run(m, next)
	}
}

func typeText(m *model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func press(m *model, k string) tea.Cmd {
	codes := map[string]tea.KeyPressMsg{
		"enter":  {Code: tea.KeyEnter},
		"esc":    {Code: tea.KeyEscape},
		"ctrl+s": {Code: 's', Mod: tea.ModCtrl},
		"ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
		"y":      {Code: 'y', Text: "y"},
		"a":      {Code: 'a', Text: "a"},
		"n":      {Code: 'n', Text: "n"},
	}
	_, cmd := m.Update(codes[k])
	return cmd
}

func apply(m *model, evs ...eventwire.Event) {
	for _, ev := range evs {
		m.tr.Apply(ev)
	}
	m.commit()
}

// The scrollback takes rows in the order they happened: a finished block of a
// streaming answer goes early, a call still running holds back what follows it.
func TestCommitKeepsTheOrderTheConversationHappenedIn(t *testing.T) {
	m, _ := testModel(t)
	m.tr.AddUser("go")
	apply(m, eventwire.Event{Kind: "turn_started"}, eventwire.Event{Kind: "text", Text: "first block\n\nstill writ"})
	say := m.tr.Items[1]
	if !m.committed[m.tr.Items[0].ID] || m.committed[say.ID] || m.sayShown[say.ID] != len("first block\n\n") {
		t.Fatalf("committed=%v shown=%v", m.committed, m.sayShown)
	}
	apply(m,
		eventwire.Event{Kind: "tool_dispatch", Tool: &eventwire.Tool{ID: "a", Name: "bash"}},
		eventwire.Event{Kind: "tool_dispatch", Tool: &eventwire.Tool{ID: "b", Name: "bash"}},
		eventwire.Event{Kind: "tool_result", Tool: &eventwire.Tool{ID: "b", Output: "ok"}},
	)
	a, b := m.tr.Items[2], m.tr.Items[3]
	if !m.committed[say.ID] || m.committed[a.ID] || m.committed[b.ID] {
		t.Fatalf("a finished call jumped a running one: %v", m.committed)
	}
	apply(m, eventwire.Event{Kind: "tool_result", Tool: &eventwire.Tool{ID: "a", Output: "ok"}})
	if !m.committed[a.ID] || !m.committed[b.ID] {
		t.Fatalf("settled calls were not committed: %v", m.committed)
	}
	if got := renderItem(&m.tr.Items[1], 80, m.sayShown[say.ID]); strings.Contains(got, "first block") {
		t.Fatalf("the answer's printed block was printed again: %q", got)
	}
}

// Input waiting in the queue stays on screen but does not hold back the rows
// that come after it.
func TestQueuedInputDoesNotHoldTheScrollback(t *testing.T) {
	m, _ := testModel(t)
	apply(m, eventwire.Event{Kind: "turn_started"})
	m.tr.AddQueued("later", false)
	apply(m, eventwire.Event{Kind: "message", Text: "done"})
	if !m.committed[m.tr.Items[1].ID] || m.committed[m.tr.Items[0].ID] {
		t.Fatalf("committed = %v, items %+v", m.committed, m.tr.Items)
	}
	if v := m.View(); !strings.Contains(v.Content, "later") {
		t.Fatalf("queued input left the screen: %q", v.Content)
	}
}

func TestEnterSubmitsWhenIdleAndQueuesWhileRunning(t *testing.T) {
	m, k := testModel(t)
	typeText(m, "hello")
	run(m, press(m, "enter"))
	apply(m, eventwire.Event{Kind: "turn_started"})
	typeText(m, "and tests")
	run(m, press(m, "enter"))
	typeText(m, "stop, use make")
	run(m, press(m, "ctrl+s"))
	calls := strings.Join(k.seen(), "\n")
	for _, want := range []string{
		`POST /submit {"input":"hello"}`,
		`POST /inbox/items {"input":"and tests","intent":"followup"}`,
		`POST /inbox/items {"input":"stop, use make","intent":"steer"}`,
	} {
		if !strings.Contains(calls, want) {
			t.Fatalf("missing %q in\n%s", want, calls)
		}
	}
	queued := m.tr.Items[len(m.tr.Items)-1]
	if !queued.Pending || queued.QueueID != "q-7" {
		t.Fatalf("queued row = %+v", queued)
	}
	run(m, press(m, "esc"))
	if !strings.Contains(strings.Join(k.seen(), "\n"), "POST /cancel") {
		t.Fatal("esc did not cancel the running turn")
	}
}

// An approval takes the answers the host said it honours, and no others.
func TestApprovalKeysAnswerOnlyWhatTheHostAllows(t *testing.T) {
	m, k := testModel(t)
	apply(m, eventwire.Event{Kind: "approval_request", Approval: &eventwire.Approval{ID: "ap1", Tool: "bash", Subject: "rm x"}})
	run(m, press(m, "a"))
	if m.tr.OpenPrompt() == nil {
		t.Fatal("a session grant the host does not offer was accepted")
	}
	run(m, press(m, "y"))
	if m.tr.OpenPrompt() != nil {
		t.Fatal("y did not settle the approval")
	}
	if calls := strings.Join(k.seen(), "\n"); !strings.Contains(calls, `POST /approve {"allow":true,"id":"ap1","persist":false,"session":false}`) {
		t.Fatalf("approve call missing:\n%s", calls)
	}
}

func TestLargePasteFoldsAndExpandsOnSend(t *testing.T) {
	m, k := testModel(t)
	big := strings.Repeat("line\n", 10)
	m.Update(tea.PasteMsg{Content: big})
	if v := m.composer.Value(); v != "[Pasted text #1 +11 lines]" {
		t.Fatalf("composer = %q", v)
	}
	run(m, press(m, "enter"))
	raw, _ := json.Marshal(map[string]string{"input": big})
	if calls := strings.Join(k.seen(), "\n"); !strings.Contains(calls, string(raw)) {
		t.Fatalf("the paste did not go whole:\n%s", calls)
	}
}

func TestCtrlCTwiceQuitsWhenIdle(t *testing.T) {
	m, _ := testModel(t)
	if cmd := press(m, "ctrl+c"); cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("one ctrl+c quit")
		}
	}
	cmd := press(m, "ctrl+c")
	if cmd == nil {
		t.Fatal("second ctrl+c did nothing")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Fatal("second ctrl+c did not quit")
	}
}
