package cli

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"reasonix/internal/agent"
	"reasonix/internal/control"
	"reasonix/internal/event"
	"reasonix/internal/i18n"
	"reasonix/internal/provider"
	"reasonix/internal/session"
)

// newCanonicalTakeoverTUI builds an exclusive-session TUI over a fresh
// sessions-v4 catalog sharing the CLI's cached session service, plus a second
// ("held") identity the fake serve pretends to release.
func newCanonicalTakeoverTUI(t *testing.T) (*chatTUI, *control.Controller, *session.Service, session.SessionRef) {
	t.Helper()
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	service := cliSessionService(sessionDir)
	if service == nil {
		t.Fatal("session service unavailable for test workspace")
	}
	ctrl := newOwnedTestController(t, control.Options{
		Executor:   agent.New(nil, nil, agent.NewSession("system"), agent.Options{}, event.Discard),
		SessionDir: sessionDir, SessionService: service, ExclusiveSession: true,
	})
	if _, err := ctrl.BindFreshSession(t.Context(), "fresh-cli"); err != nil {
		t.Fatal(err)
	}
	held, err := service.Create(t.Context(), session.CreateOptions{SessionID: "held"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(t.Context(), held.Ref()); err != nil {
		t.Fatal(err)
	}
	m := newTestChatTUI()
	m.ctrl = ctrl
	m.leases = control.NewSessionLeaseKeeper()
	t.Cleanup(m.leases.Release)
	m.takeover = newCLITakeoverManager(nil, m.leases)
	t.Cleanup(func() {
		ctrl.Close()
		_ = service.CloseAll(context.Background())
	})
	return &m, ctrl, service, held.Ref()
}

// fakeCanonicalServe impersonates the resident serve: it grants the identity
// handoff, accepts mirrored frames, and can flag a reclaim.
type fakeCanonicalServe struct {
	mu          sync.Mutex
	base        string
	handoffBody map[string]any
	// routes, when set, lists every route the serve grants; the grant echoes
	// the requested route and a distinct mirror id. Unlisted routes are
	// refused like a serve that does not hold the session.
	routes     map[string]bool
	handoffs   int
	framesPath []string
	mirrorEnd  []string
	reclaim    atomic.Bool
}

func newFakeCanonicalServe(t *testing.T, route string) *fakeCanonicalServe {
	t.Helper()
	return newFakeCanonicalServeRoutes(t, route)
}

func newFakeCanonicalServeRoutes(t *testing.T, routes ...string) *fakeCanonicalServe {
	t.Helper()
	f := &fakeCanonicalServe{}
	f.handoffBody = map[string]any{
		"sessionPath": routes[0], "mirrorId": "mirror-1", "handoffId": "handoff-1",
		"returnHandoffId": "return-1", "sourceWriterId": "serve-writer",
		"targetWriterId": agent.SessionWriterID(), "status": "handed_off",
	}
	if len(routes) > 1 {
		f.routes = make(map[string]bool, len(routes))
		for _, route := range routes {
			f.routes[route] = true
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			w.WriteHeader(http.StatusNoContent)
		case "/handoff":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			requested, _ := body["sessionPath"].(string)
			f.mu.Lock()
			f.handoffs++
			f.handoffBody["__received"] = body
			response := make(map[string]any, len(f.handoffBody))
			maps.Copy(response, f.handoffBody)
			if f.routes != nil {
				if !f.routes[requested] {
					f.mu.Unlock()
					http.Error(w, "session is not held by this serve process", http.StatusConflict)
					return
				}
				response["sessionPath"] = requested
				response["mirrorId"] = "mirror-" + strconv.Itoa(f.handoffs)
			}
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(response)
		case "/external/frames":
			var body struct {
				SessionPath string `json:"sessionPath"`
				MirrorID    string `json:"mirrorId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.framesPath = append(f.framesPath, body.SessionPath)
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"reclaimRequested": f.reclaim.Load(), "reclaimMode": "wait"})
		case "/mirror-end":
			var body struct {
				SessionPath string `json:"sessionPath"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.mirrorEnd = append(f.mirrorEnd, body.SessionPath)
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	f.base = srv.URL
	return f
}

func (f *fakeCanonicalServe) mirrorEnds() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.mirrorEnd...)
}

func (f *fakeCanonicalServe) handoffCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.handoffs
}

func withFakeCanonicalDiscovery(t *testing.T, base string) {
	t.Helper()
	previous := discoverCLIServesForTakeover
	discoverCLIServesForTakeover = func() []cliServeRecord {
		return []cliServeRecord{{pid: 1, base: base, token: "test-token"}}
	}
	t.Cleanup(func() { discoverCLIServesForTakeover = previous })
}

// TestCanonicalTakeoverCommandTakesOverIdentity proves the /takeover command
// against an identity route: the grant is validated, the controller attaches
// through OpenSession, and the mirror manager activates on the route key.
func TestCanonicalTakeoverCommandTakesOverIdentity(t *testing.T) {
	route := cliCanonicalRoute("held")
	fake := newFakeCanonicalServe(t, route)
	withFakeCanonicalDiscovery(t, fake.base)
	m, ctrl, _, held := newCanonicalTakeoverTUI(t)

	m.runCanonicalTakeoverCommand(route)

	if got := m.pendingTakeoverPath; got != "" {
		t.Fatalf("pending takeover target = %q after success", got)
	}
	if ref, bound := ctrl.SessionRef(); !bound || ref != held {
		t.Fatalf("controller ref after takeover = %+v (bound %v), want %+v", ref, bound, held)
	}
	binding, _, _, _ := m.takeover.snapshot()
	if binding == nil || binding.path != route || !binding.canonical {
		t.Fatalf("mirror binding = %+v, want canonical route %q", binding, route)
	}
	fake.mu.Lock()
	received, _ := fake.handoffBody["__received"].(map[string]any)
	fake.mu.Unlock()
	if received == nil || received["sessionPath"] != route || received["targetWriterId"] != agent.SessionWriterID() {
		t.Fatalf("handoff request = %+v", received)
	}
	if err := m.takeover.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestCanonicalTakeoverCommandReportsRefusedGrant proves a refusing serve
// surfaces the failure without touching the controller's session.
func TestCanonicalTakeoverCommandReportsRefusedGrant(t *testing.T) {
	route := cliCanonicalRoute("held")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "session is not held by this serve process", http.StatusConflict)
		}
	}))
	defer srv.Close()
	withFakeCanonicalDiscovery(t, srv.URL)
	m, ctrl, _, held := newCanonicalTakeoverTUI(t)
	// Model the refusal that matters: another runtime really owns the writer,
	// so the refused grant is the only way in.
	holder, err := session.NewService("local", session.NewFilesystemPersistence(session.RootForLegacyDir(ctrl.SessionDir())))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.CloseAll(context.Background()) })
	if _, err := holder.Open(t.Context(), held); err != nil {
		t.Fatal(err)
	}

	m.runCanonicalTakeoverCommand(route)

	if ref, bound := ctrl.SessionRef(); !bound || ref.SessionID != "fresh-cli" {
		t.Fatalf("controller ref after refused takeover = %+v (bound %v), want the original fresh session", ref, bound)
	}
	if binding, _, _, _ := m.takeover.snapshot(); binding != nil {
		t.Fatalf("mirror binding activated despite refusal: %+v", binding)
	}
}

// TestCanonicalReclaimYieldsWithoutLegacyLease proves the yield half: a
// reclaim signal against a canonical binding returns the mirror without any
// legacy path-lease reservation, and mirror-end carries the identity route.
func TestCanonicalReclaimYieldsWithoutLegacyLease(t *testing.T) {
	route := cliCanonicalRoute("held")
	fake := newFakeCanonicalServe(t, route)
	m, ctrl, _, held := newCanonicalTakeoverTUI(t)
	withFakeCanonicalDiscovery(t, fake.base)
	m.runCanonicalTakeoverCommand(route)
	if ref, bound := ctrl.SessionRef(); !bound || ref != held {
		t.Fatalf("takeover did not attach: %+v bound=%v", ref, bound)
	}

	exited := make(chan struct{}, 1)
	m.takeover.SetYieldCallback(func() { exited <- struct{}{} })
	fake.reclaim.Store(true)
	m.takeover.Emit(event.Event{Kind: event.Text, Text: "answer"})
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("reclaim did not yield the canonical mirror")
	}
	if !m.takeover.Returned() {
		t.Fatal("canonical mirror not marked returned")
	}
	fake.mu.Lock()
	ends := append([]string(nil), fake.mirrorEnd...)
	fake.mu.Unlock()
	if len(ends) != 1 || ends[0] != route {
		t.Fatalf("mirror-end requests = %v, want one for %q", ends, route)
	}
}

// TestCanonicalReclaimKeepsTUIAliveAndSwitchesSession proves that reclaiming
// one identity releases its writer while leaving the CLI process available to
// resume another identity.
func TestCanonicalReclaimKeepsTUIAliveAndSwitchesSession(t *testing.T) {
	route := cliCanonicalRoute("held")
	fake := newFakeCanonicalServe(t, route)
	withFakeCanonicalDiscovery(t, fake.base)
	m, ctrl, service, held := newCanonicalTakeoverTUI(t)
	if err := ctrl.RecordSessionMessages(t.Context(), "reclaim-test", []provider.Message{{
		ID: agent.NewMessageID(), Role: provider.RoleUser, Content: "existing session",
	}}); err != nil {
		t.Fatal(err)
	}
	m.runCanonicalTakeoverCommand(route)

	yielded := make(chan struct{}, 1)
	m.takeover.SetYieldCallback(func() { yielded <- struct{}{} })
	fake.reclaim.Store(true)
	m.takeover.Emit(event.Event{Kind: event.Text, Text: "answer"})
	select {
	case <-yielded:
	case <-time.After(5 * time.Second):
		t.Fatal("reclaim did not yield the identity")
	}

	if next, cmd := m.Update(tuiSessionReclaimedMsg{}); cmd != nil {
		t.Fatalf("reclaim updated TUI with quit command %T", cmd)
	} else {
		updated := next.(chatTUI)
		m = &updated
	}
	// The reclaimed conversation stays rendered with a notice instead of a
	// forced chooser; only the switch/takeover/exit commands are accepted.
	if !m.sessionReclaimed || m.resumePick != nil {
		t.Fatalf("reclaim state = %v picker=%v, want live TUI on the reclaimed session", m.sessionReclaimed, m.resumePick != nil)
	}
	if reclaimInputAllowed("hello") {
		t.Fatal("reclaim gate accepted plain text")
	}
	if !reclaimInputAllowed("/resume 2") {
		t.Fatal("reclaim gate rejected /resume")
	}
	dir, err := service.SessionDir(t.Context(), held)
	if err != nil {
		t.Fatal(err)
	}
	if session.ProbeWriterHeld(dir) {
		t.Fatal("reclaimed identity writer lock is still held by the CLI")
	}

	// /resume opens the chooser on demand and switching clears the reclaim state.
	m.runResumeCommand("/resume")
	if m.resumePick == nil {
		t.Fatal("/resume after reclaim did not open the session chooser")
	}
	entries := m.resumePick.entries
	for i, entry := range entries {
		if entry.target.canonical() && entry.target.ref.SessionID == "fresh-cli" {
			m.resumePick.sel = i
			if m.resumePick.quick != nil {
				m.resumePick.quick.selected = i
			}
			break
		}
	}
	if m.resumePick.sel < 0 || m.resumePick.sel >= len(entries) || !entries[m.resumePick.sel].target.canonical() || entries[m.resumePick.sel].target.ref.SessionID != "fresh-cli" {
		t.Fatalf("resume picker did not expose fresh-cli: %+v", entries)
	}
	// Confirm through the same key path used by the live picker. Calling
	// applyResumePick directly would miss routing or quick-picker regressions.
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("resume after reclaim returned unexpected command %T", cmd)
	}
	updated := next.(chatTUI)
	m = &updated
	if ref, bound := ctrl.SessionRef(); !bound || ref.SessionID != "fresh-cli" {
		t.Fatalf("controller after reclaim resume = %+v bound=%v, want fresh-cli", ref, bound)
	}
	if m.sessionReclaimed || m.takeover.Returned() {
		t.Fatal("reclaim marker remained after switching to another session")
	}
}

// TestCanonicalReclaimCanTakeSameSessionAgain covers the complete ownership
// round trip: CLI takeover, desktop reclaim, then CLI takeover once more.
func TestCanonicalReclaimCanTakeSameSessionAgain(t *testing.T) {
	route := cliCanonicalRoute("held")
	fake := newFakeCanonicalServe(t, route)
	withFakeCanonicalDiscovery(t, fake.base)
	m, ctrl, _, held := newCanonicalTakeoverTUI(t)
	m.runCanonicalTakeoverCommand(route)

	yielded := make(chan struct{}, 1)
	m.takeover.SetYieldCallback(func() { yielded <- struct{}{} })
	fake.reclaim.Store(true)
	m.takeover.Emit(event.Event{Kind: event.Text, Text: "answer"})
	select {
	case <-yielded:
	case <-time.After(5 * time.Second):
		t.Fatal("reclaim did not yield the identity")
	}
	if next, cmd := m.Update(tuiSessionReclaimedMsg{}); cmd != nil {
		t.Fatalf("reclaim updated TUI with quit command %T", cmd)
	} else {
		updated := next.(chatTUI)
		m = &updated
	}
	if _, bound := ctrl.SessionRef(); bound {
		t.Fatal("controller remained bound after desktop reclaim")
	}

	fake.reclaim.Store(false)
	// The user-facing path: after a reclaim the notice says "/takeover takes
	// it back" — the command must resolve the remembered reclaimed target
	// without a prior /resume (which would re-populate pendingTakeoverPath).
	m.runTakeoverCommand("/takeover")
	if ref, bound := ctrl.SessionRef(); !bound || ref != held {
		t.Fatalf("controller after second takeover = %+v bound=%v, want %+v", ref, bound, held)
	}
	if m.sessionReclaimed || m.takeover.Returned() {
		t.Fatal("second takeover left the CLI in reclaimed mode")
	}
}

// TestDiscoverCLIServesSkipsDeadPIDs pins the discovery prune: state files
// whose recorded process is gone must not surface as takeover candidates —
// dialing their stale ports only produces connection-refused noise.
func TestDiscoverCLIServesSkipsDeadPIDs(t *testing.T) {
	home := t.TempDir()
	stateDir := filepath.Join(home, "remote")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REASONIX_HOME", home)
	write := func(slug, addr string, pid int) {
		t.Helper()
		state := map[string]any{"pid": pid, "addr": addr, "workspace": "/tmp/" + slug}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stateDir, "serve-"+slug+".json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stateDir, "serve-"+slug+".token"), []byte("tok-"+slug), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("dead-one", "127.0.0.1:44173", 111)
	write("alive-one", "127.0.0.1:33863", 222)

	previous := cliServeProcessAlive
	cliServeProcessAlive = func(pid int) bool { return pid == 222 }
	t.Cleanup(func() { cliServeProcessAlive = previous })

	records := discoverCLIServes()
	if len(records) != 1 || records[0].base != "http://127.0.0.1:33863" || records[0].token != "tok-alive-one" {
		t.Fatalf("discovery after prune = %+v, want only the alive record", records)
	}
}

// resumeEntryIndex returns the 1-based /resume index of the row matching the
// predicate, or 0 when absent.
func resumeEntryIndex(entries []resumeEntry, match func(resumeEntry) bool) int {
	for i, entry := range entries {
		if match(entry) {
			return i + 1
		}
	}
	return 0
}

func assertMirrorLeft(t *testing.T, m *chatTUI, fake *fakeCanonicalServe, route string) {
	t.Helper()
	if binding, _, _, _ := m.takeover.snapshot(); binding != nil {
		t.Fatalf("mirror binding still active after leaving %q: %+v", route, binding)
	}
	if ends := fake.mirrorEnds(); len(ends) != 1 || ends[0] != route {
		t.Fatalf("mirror-end requests = %v, want exactly one for %q", ends, route)
	}
	if m.takeover.Returned() {
		t.Fatal("switching away from a mirror left the manager in returned state")
	}
}

// TestResumeCanonicalSessionReturnsActiveCanonicalMirror covers /resume to a
// final-format session while this CLI mirrors another canonical identity for
// the desktop: the mirror must end so the desktop tab regains its writer and
// stops receiving the next session's frames under the old mirror id.
func TestResumeCanonicalSessionReturnsActiveCanonicalMirror(t *testing.T) {
	route := cliCanonicalRoute("held")
	fake := newFakeCanonicalServe(t, route)
	withFakeCanonicalDiscovery(t, fake.base)
	m, ctrl, _, held := newCanonicalTakeoverTUI(t)
	m.runCanonicalTakeoverCommand(route)
	if ref, bound := ctrl.SessionRef(); !bound || ref != held {
		t.Fatalf("takeover did not attach: %+v bound=%v", ref, bound)
	}
	createCanonicalTestSession(t, session.RootForLegacyDir(ctrl.SessionDir()), "third", "third conversation")

	idx := resumeEntryIndex(resumeEntries(ctrl.SessionDir()), func(entry resumeEntry) bool {
		return entry.target.canonical() && entry.target.ref.SessionID == "third"
	})
	if idx == 0 {
		t.Fatal("third canonical session missing from /resume list")
	}
	m.runResumeCommand("/resume " + strconv.Itoa(idx))

	if ref, bound := ctrl.SessionRef(); !bound || ref.SessionID != "third" {
		t.Fatalf("controller after /resume = %+v bound=%v, want third", ref, bound)
	}
	assertMirrorLeft(t, m, fake, route)
}

// TestCanonicalTakeoverOfSecondIdentityReturnsFirstMirror covers /takeover of
// a second final-format identity while the first is still mirrored: Activate
// must not overwrite the live binding without ending the first mirror.
func TestCanonicalTakeoverOfSecondIdentityReturnsFirstMirror(t *testing.T) {
	routeA, routeB := cliCanonicalRoute("held"), cliCanonicalRoute("second")
	fake := newFakeCanonicalServeRoutes(t, routeA, routeB)
	withFakeCanonicalDiscovery(t, fake.base)
	m, ctrl, service, held := newCanonicalTakeoverTUI(t)
	second, err := service.Create(t.Context(), session.CreateOptions{SessionID: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(t.Context(), second.Ref()); err != nil {
		t.Fatal(err)
	}
	m.runCanonicalTakeoverCommand(routeA)
	if ref, bound := ctrl.SessionRef(); !bound || ref != held {
		t.Fatalf("first takeover did not attach: %+v bound=%v", ref, bound)
	}

	m.runCanonicalTakeoverCommand(routeB)

	if ref, bound := ctrl.SessionRef(); !bound || ref != second.Ref() {
		t.Fatalf("controller after second takeover = %+v bound=%v, want second", ref, bound)
	}
	binding, _, _, _ := m.takeover.snapshot()
	if binding == nil || binding.path != routeB || !binding.canonical {
		t.Fatalf("mirror binding = %+v, want the second identity %q", binding, routeB)
	}
	if ends := fake.mirrorEnds(); len(ends) != 1 || ends[0] != routeA {
		t.Fatalf("mirror-end requests = %v, want exactly one for the first identity %q", ends, routeA)
	}
	if fake.handoffCount() != 2 {
		t.Fatalf("handoff requests = %d, want one per takeover", fake.handoffCount())
	}
}

// TestResumeLegacySessionReturnsActiveCanonicalMirror covers /resume to a
// legacy transcript while mirroring a canonical identity. The live keeper holds
// no lease after a canonical attach, so the detached source keeper carries no
// reverse reservation to publish; the switch must still end the mirror instead
// of failing with "no detached session lease held" or leaking it.
func TestResumeLegacySessionReturnsActiveCanonicalMirror(t *testing.T) {
	route := cliCanonicalRoute("held")
	fake := newFakeCanonicalServe(t, route)
	withFakeCanonicalDiscovery(t, fake.base)
	m, ctrl, _, held := newCanonicalTakeoverTUI(t)
	m.runCanonicalTakeoverCommand(route)
	if ref, bound := ctrl.SessionRef(); !bound || ref != held {
		t.Fatalf("takeover did not attach: %+v bound=%v", ref, bound)
	}
	legacy := saveQueryTestSession(t, ctrl.SessionDir(), "legacy-target.jsonl", "LEGACY-TARGET-PROMPT")

	idx := resumeEntryIndex(resumeEntries(ctrl.SessionDir()), func(entry resumeEntry) bool {
		return !entry.target.canonical() && entry.target.path == legacy
	})
	if idx == 0 {
		t.Fatal("legacy transcript missing from /resume list")
	}
	m.runResumeCommand("/resume " + strconv.Itoa(idx))

	ref, bound := ctrl.SessionRef()
	if !bound || ref == held {
		t.Fatalf("controller after /resume = %+v bound=%v, want the imported legacy transcript", ref, bound)
	}
	loaded := false
	for _, msg := range ctrl.History() {
		loaded = loaded || msg.Content == "LEGACY-TARGET-PROMPT"
	}
	if !loaded {
		t.Fatal("history not loaded from the legacy target")
	}
	assertMirrorLeft(t, m, fake, route)
}

// TestResumeCanonicalSessionReturnsLegacyMirrorReservation covers the legacy
// half of the leave step: switching from a mirrored legacy transcript to a
// final-format session publishes the transcript's reverse reservation for the
// serve and ends the mirror before the controller publishes the new identity.
func TestResumeCanonicalSessionReturnsLegacyMirrorReservation(t *testing.T) {
	m, ctrl, service, _ := newCanonicalTakeoverTUI(t)
	legacy := saveQueryTestSession(t, ctrl.SessionDir(), "mirrored-legacy.jsonl", "mirrored legacy")
	if err := m.leases.Rebind(legacy); err != nil {
		t.Fatal(err)
	}
	fake := newFakeCanonicalServe(t, legacy)
	m.takeover.AttachController(ctrl)
	m.takeover.Activate(&cliTakeoverBinding{
		path: legacy, record: cliServeRecord{base: fake.base}, client: &http.Client{},
		grant: cliTakeoverGrant{MirrorID: "mirror-legacy", SourceWriterID: "serve-writer", ReturnHandoffID: "return-legacy"},
	})
	third, err := service.Create(t.Context(), session.CreateOptions{SessionID: "third"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(t.Context(), third.Ref()); err != nil {
		t.Fatal(err)
	}

	if err := m.commitCanonicalSessionSwitch(third.Ref()); err != nil {
		t.Fatalf("commitCanonicalSessionSwitch: %v", err)
	}

	if ref, bound := ctrl.SessionRef(); !bound || ref != third.Ref() {
		t.Fatalf("controller after switch = %+v bound=%v, want third", ref, bound)
	}
	assertMirrorLeft(t, m, fake, legacy)
	info, err := agent.LoadSessionLeaseInfo(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if info == nil || info.HandoffTo != "serve-writer" || info.HandoffID != "return-legacy" {
		t.Fatalf("legacy reverse reservation = %+v, want the serve's return handoff", info)
	}
	if held := m.leases.HeldPath(); held != "" {
		t.Fatalf("keeper still holds %q after handing the legacy transcript back", held)
	}
}

// TestResumeCanonicalSessionHeldElsewhereKeepsMirror pins failure atomicity of
// the canonical switch: when the target's writer belongs to another runtime,
// the current mirror, controller binding, and lease stay exactly as they were.
func TestResumeCanonicalSessionHeldElsewhereKeepsMirror(t *testing.T) {
	route := cliCanonicalRoute("held")
	fake := newFakeCanonicalServe(t, route)
	withFakeCanonicalDiscovery(t, fake.base)
	m, ctrl, _, held := newCanonicalTakeoverTUI(t)
	m.runCanonicalTakeoverCommand(route)
	other, err := session.NewService("local", session.NewFilesystemPersistence(session.RootForLegacyDir(ctrl.SessionDir())))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.CloseAll(context.Background()) })
	busy, err := other.Create(t.Context(), session.CreateOptions{SessionID: "busy"})
	if err != nil {
		t.Fatal(err)
	}

	err = m.commitCanonicalSessionSwitch(session.SessionRef{HostID: busy.Ref().HostID, SessionID: "busy"})

	if !errors.Is(err, session.ErrWriterOwned) {
		t.Fatalf("switch to a held session returned %v, want ErrWriterOwned", err)
	}
	if ref, bound := ctrl.SessionRef(); !bound || ref != held {
		t.Fatalf("controller moved to %+v (bound %v) despite the refused switch", ref, bound)
	}
	binding, _, _, _ := m.takeover.snapshot()
	if binding == nil || binding.path != route {
		t.Fatalf("mirror binding = %+v after a refused switch, want %q still active", binding, route)
	}
	if ends := fake.mirrorEnds(); len(ends) != 0 {
		t.Fatalf("mirror-end requests = %v after a refused switch, want none", ends)
	}
}

// TestCanonicalTakeoverDoesNotRetryOnServeVerdict pins the retry rule: a serve
// that answers — here the wait-mode "still running" verdict, which already
// cost one bounded drain window — is not asked again through a second
// discovery pass, so the synchronous worst case is one round, not two.
func TestCanonicalTakeoverDoesNotRetryOnServeVerdict(t *testing.T) {
	var handoffs atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			w.WriteHeader(http.StatusNoContent)
		case "/handoff":
			handoffs.Add(1)
			http.Error(w, "session is still running; retry with mode=interrupt", http.StatusConflict)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	var discoveries atomic.Int32
	previous := discoverCLIServesForTakeover
	discoverCLIServesForTakeover = func() []cliServeRecord {
		discoveries.Add(1)
		return []cliServeRecord{{pid: 1, base: srv.URL, token: "test-token"}}
	}
	t.Cleanup(func() { discoverCLIServesForTakeover = previous })

	binding, err := cliTakeoverIdentityHeldSession(cliCanonicalRoute("held"), nil)

	if binding != nil || err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("takeover = %+v, %v; want the serve's verdict", binding, err)
	}
	if cliServeUnreachable(err) {
		t.Fatal("an HTTP verdict was classified as a transport failure")
	}
	if got := handoffs.Load(); got != 1 {
		t.Fatalf("handoff requests = %d, want exactly 1 after a verdict", got)
	}
	if got := discoveries.Load(); got != 1 {
		t.Fatalf("discovery passes = %d, want 1 after a verdict", got)
	}
}

// TestCanonicalTakeoverRediscoversAfterTransportFailure keeps the one
// re-discovery pass for the case it exists for: every recorded serve was
// unreachable (a desktop reconnect respawned it), and the fresh state file
// names the live serve.
func TestCanonicalTakeoverRediscoversAfterTransportFailure(t *testing.T) {
	route := cliCanonicalRoute("held")
	fake := newFakeCanonicalServe(t, route)
	var discoveries atomic.Int32
	previous := discoverCLIServesForTakeover
	discoverCLIServesForTakeover = func() []cliServeRecord {
		if discoveries.Add(1) == 1 {
			// Nothing listens on port 1: the dial fails at the transport.
			return []cliServeRecord{{pid: 1, base: "http://127.0.0.1:1", token: "stale-token"}}
		}
		return []cliServeRecord{{pid: 2, base: fake.base, token: "test-token"}}
	}
	t.Cleanup(func() { discoverCLIServesForTakeover = previous })

	binding, err := cliTakeoverIdentityHeldSession(route, nil)

	if err != nil || binding == nil || binding.path != route {
		t.Fatalf("takeover = %+v, %v; want a grant from the re-discovered serve", binding, err)
	}
	if got := discoveries.Load(); got != 2 {
		t.Fatalf("discovery passes = %d, want 2 (one after the transport failure)", got)
	}
	if fake.handoffCount() != 1 {
		t.Fatalf("handoff requests to the live serve = %d, want 1", fake.handoffCount())
	}
}

// TestCanonicalTakeoverOfFreeSessionResumes covers the promise the reclaim
// notice makes after the desktop closed the session it took back: no runtime
// holds the identity any more, so there is nothing to hand over and
// /takeover resumes it instead of failing with "no resident serve holds this
// session". Both shapes of "closed" are covered: the serve exited, and a
// resident serve that no longer holds the session and refuses.
func TestCanonicalTakeoverOfFreeSessionResumes(t *testing.T) {
	t.Run("the serve exited", func(t *testing.T) {
		previous := discoverCLIServesForTakeover
		discoverCLIServesForTakeover = func() []cliServeRecord { return nil }
		t.Cleanup(func() { discoverCLIServesForTakeover = previous })
		m, ctrl, _, held := newCanonicalTakeoverTUI(t)

		m.runCanonicalTakeoverCommand(cliCanonicalRoute("held"))

		if ref, bound := ctrl.SessionRef(); !bound || ref != held {
			t.Fatalf("controller after /takeover of a free session = %+v bound=%v, want %+v", ref, bound, held)
		}
		if binding, _, _, _ := m.takeover.snapshot(); binding != nil {
			t.Fatalf("resuming a free session activated a mirror: %+v", binding)
		}
	})
	t.Run("a resident serve no longer holds it", func(t *testing.T) {
		route := cliCanonicalRoute("held")
		fake := newFakeCanonicalServe(t, route)
		withFakeCanonicalDiscovery(t, fake.base)
		m, ctrl, _, held := newCanonicalTakeoverTUI(t)
		m.runCanonicalTakeoverCommand(route)
		yielded := make(chan struct{}, 1)
		m.takeover.SetYieldCallback(func() { yielded <- struct{}{} })
		fake.reclaim.Store(true)
		m.takeover.Emit(event.Event{Kind: event.Text, Text: "answer"})
		select {
		case <-yielded:
		case <-time.After(5 * time.Second):
			t.Fatal("reclaim did not yield the identity")
		}
		next, _ := m.Update(tuiSessionReclaimedMsg{})
		updated := next.(chatTUI)
		m = &updated
		// The desktop closed the tab: the serve stays resident but refuses,
		// and nobody holds the writer.
		refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/auth/token" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Error(w, "session is not held by this serve process", http.StatusConflict)
		}))
		defer refusing.Close()
		withFakeCanonicalDiscovery(t, refusing.URL)

		m.runTakeoverCommand("/takeover")

		if ref, bound := ctrl.SessionRef(); !bound || ref != held {
			t.Fatalf("controller after /takeover = %+v bound=%v, want %+v resumed", ref, bound, held)
		}
		if m.sessionReclaimed || m.takeover.Returned() {
			t.Fatal("/takeover of the freed session left the CLI in reclaimed mode")
		}
		if binding, _, _, _ := m.takeover.snapshot(); binding != nil {
			t.Fatalf("resuming a free session activated a mirror: %+v", binding)
		}
	})
}

// TestCanonicalTakeoverOfActiveSessionIsRejected keeps /takeover from
// "resuming" the identity this controller already writes.
func TestCanonicalTakeoverOfActiveSessionIsRejected(t *testing.T) {
	previous := discoverCLIServesForTakeover
	discoverCLIServesForTakeover = func() []cliServeRecord { return nil }
	t.Cleanup(func() { discoverCLIServesForTakeover = previous })
	m, ctrl, _, _ := newCanonicalTakeoverTUI(t)

	m.runCanonicalTakeoverCommand(cliCanonicalRoute("fresh-cli"))

	if ref, bound := ctrl.SessionRef(); !bound || ref.SessionID != "fresh-cli" {
		t.Fatalf("controller = %+v bound=%v, want the active session untouched", ref, bound)
	}
	out := strings.Join(m.transcript, "\n")
	if !strings.Contains(out, i18n.M.ResumeAlreadyActive) {
		t.Fatalf("transcript missing the already-active notice:\n%s", out)
	}
	if strings.Contains(out, i18n.M.ResumedTitle) {
		t.Fatalf("/takeover of the active session replayed the transcript:\n%s", out)
	}
}
