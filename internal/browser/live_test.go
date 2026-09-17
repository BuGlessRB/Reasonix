package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"reasonix/internal/testenv"
)

// liveSession starts a real browser when REASONIX_LIVE_BROWSER names one (or
// is "1" to discover an installed one). Normal runs skip: CI machines differ in
// whether any Chromium is installed.
func liveSession(t *testing.T, headless bool) *Session {
	t.Helper()
	want := os.Getenv("REASONIX_LIVE_BROWSER")
	if want == "" {
		t.Skip("set REASONIX_LIVE_BROWSER=1 or to a browser executable to run live browser tests")
	}
	configured := want
	if want == "1" {
		configured = ""
	}
	exe, err := Discover(configured)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	root := testenv.TempDir(t)
	s := NewSession(Config{
		Launch: LaunchSpec{Executable: exe, ProfileDir: testenv.TempDir(t), Headless: headless},
		Roots:  []string{root},
	})
	t.Cleanup(s.Close)
	return s
}

func liveServer(t *testing.T, pages map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, body := range pages {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(body))
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestLiveOpenLoadsAPage(t *testing.T) {
	s := liveSession(t, true)
	srv := liveServer(t, map[string]string{"/": `<title>Hello</title><h1>Hi</h1>`})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	info, err := s.Open(ctx, srv.URL+"/", "", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if info.Title != "Hello" || info.ID != "t1" {
		t.Fatalf("info = %+v", info)
	}
}

func findRef(t *testing.T, snap Snapshot, needle string) string {
	t.Helper()
	for _, line := range snap.Lines {
		if strings.Contains(line, needle) {
			start := strings.Index(line, "[e")
			end := strings.Index(line[start:], "]")
			if start >= 0 && end > 0 {
				return line[start+1 : start+end]
			}
		}
	}
	t.Fatalf("no line containing %q with a ref in:\n%s", needle, strings.Join(snap.Lines, "\n"))
	return ""
}

const formPage = `<!doctype html><title>Form</title>
<h1>Sign up</h1>
<label>Email <input id="email" type="email"></label>
<label>Plan <select id="plan"><option value="free">Free</option><option value="pro">Pro</option></select></label>
<button id="go" onclick="document.getElementById('out').textContent = 'hello ' + document.getElementById('email').value + ' ' + document.getElementById('plan').value; console.error('boom')">Submit</button>
<p id="out"></p>
<div style="position:relative;height:40px"><button id="under">Hidden</button><div style="position:absolute;inset:0;background:#fff" class="veil"></div></div>
<a href="/next">Next page</a>
<button onclick="window.open('/next')">Pop</button>
<button onclick="document.getElementById('out').textContent = confirm('sure?') ? 'yes' : 'no'">Ask</button>`

func TestLiveSnapshotAndAct(t *testing.T) {
	s := liveSession(t, true)
	srv := liveServer(t, map[string]string{"/": formPage, "/next": `<title>Next</title><h1>Arrived</h1>`})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := s.Open(ctx, srv.URL+"/", "", false); err != nil {
		t.Fatalf("Open: %v", err)
	}
	snap, err := s.Snapshot(ctx, "", "")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	t.Logf("snapshot:\n%s", strings.Join(snap.Lines, "\n"))
	email := findRef(t, snap, `textbox "Email"`)
	plan := findRef(t, snap, `combobox "Plan"`)
	submit := findRef(t, snap, `button "Submit"`)

	res, err := s.Act(ctx, "", []Step{
		{Action: "fill", Ref: email, Text: "李雷@example.com"},
		{Action: "select", Ref: plan, Values: []string{"Pro"}},
		{Action: "click", Ref: submit},
		{Action: "wait_for", Text: "hello 李雷@example.com pro"},
	})
	if err != nil {
		t.Fatalf("Act: %v (done %d, notes %v)", err, res.Done, res.Notes)
	}
	if len(res.Logs) == 0 || !strings.Contains(res.Logs[0].Text, "boom") {
		t.Fatalf("the console error did not come back with the act: %+v", res.Logs)
	}

	hidden := findRef(t, snap, `button "Hidden"`)
	_, err = s.Act(ctx, "", []Step{{Action: "click", Ref: hidden}})
	if CodeOf(err) != CodeCovered || !strings.Contains(err.Error(), "veil") {
		t.Fatalf("clicking under a veil = %v, want %s naming the veil", err, CodeCovered)
	}

	ask := findRef(t, snap, `button "Ask"`)
	res, err = s.Act(ctx, "", []Step{{Action: "click", Ref: ask}})
	if err != nil || res.Dialog == nil || res.Dialog.Type != "confirm" {
		t.Fatalf("confirm dialog: err=%v dialog=%+v", err, res.Dialog)
	}
	_, err = s.Act(ctx, "", []Step{{Action: "click", Ref: submit}})
	if CodeOf(err) != CodeDialogOpen {
		t.Fatalf("acting under an open dialog = %v, want %s", err, CodeDialogOpen)
	}
	no := false
	if _, err = s.Act(ctx, "", []Step{{Action: "dialog", Accept: &no}, {Action: "wait_for", Text: "no"}}); err != nil {
		t.Fatalf("dismiss dialog: %v", err)
	}

	pop := findRef(t, snap, `button "Pop"`)
	res, err = s.Act(ctx, "", []Step{{Action: "click", Ref: pop}, {Action: "wait", Ms: 500}})
	if err != nil || len(res.Opened) != 1 {
		t.Fatalf("popup: err=%v opened=%v", err, res.Opened)
	}
	if err := s.CloseTab(ctx, res.Opened[0]); err != nil {
		t.Fatalf("close popup: %v", err)
	}

	next := findRef(t, snap, `link "Next page"`)
	res, err = s.Act(ctx, "t1", []Step{{Action: "click", Ref: next}})
	if err != nil || !strings.HasSuffix(res.Tab.URL, "/next") || res.Tab.Title != "Next" {
		t.Fatalf("link: err=%v tab=%+v", err, res.Tab)
	}
	_, err = s.Act(ctx, "t1", []Step{{Action: "click", Ref: submit}})
	if CodeOf(err) != CodeStaleRef {
		t.Fatalf("a ref from the previous page = %v, want %s", err, CodeStaleRef)
	}
	res, err = s.Act(ctx, "t1", []Step{{Action: "back"}})
	if err != nil || strings.HasSuffix(res.Tab.URL, "/next") {
		t.Fatalf("back: err=%v tab=%+v", err, res.Tab)
	}
}

func TestLiveScreenshotCoordinatesLandOnTheElement(t *testing.T) {
	s := liveSession(t, true)
	srv := liveServer(t, map[string]string{"/": `<title>Canvas</title><body style="margin:0">
<div style="position:absolute;left:600px;top:400px;width:80px;height:60px;background:red" onclick="document.title='hit'"></div>`})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := s.Open(ctx, srv.URL+"/", "", false); err != nil {
		t.Fatalf("Open: %v", err)
	}
	url, _, err := s.Screenshot(ctx, "")
	if err != nil || !strings.HasPrefix(url, "data:image/") {
		t.Fatalf("Screenshot: %v", err)
	}
	scale := s.activeTab().screenshotScale()
	x, y := 640/scale, 430/scale
	res, err := s.Act(ctx, "", []Step{{Action: "click", X: &x, Y: &y}})
	if err != nil || res.Tab.Title != "hit" {
		t.Fatalf("coordinate click: err=%v title=%q scale=%v", err, res.Tab.Title, scale)
	}
}

func TestLiveRefusesFilesOutsideTheWorkspace(t *testing.T) {
	s := liveSession(t, true)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := s.Open(ctx, "file:///etc/hosts", "", false); CodeOf(err) != CodeURLRefused {
		t.Fatalf("file outside the workspace = %v, want %s", err, CodeURLRefused)
	}
	if _, err := s.Open(ctx, "chrome://settings", "", false); CodeOf(err) != CodeURLRefused {
		t.Fatalf("browser page = %v, want %s", err, CodeURLRefused)
	}
}

// The port endpoint is what Windows uses; driving it here keeps that path
// exercised on machines that can run the pipe.
func TestLiveWebSocketTransport(t *testing.T) {
	was := usePipe
	usePipe = false
	t.Cleanup(func() { usePipe = was })
	s := liveSession(t, true)
	srv := liveServer(t, map[string]string{"/": `<title>Socket</title><button>Go</button>`})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	info, err := s.Open(ctx, srv.URL+"/", "", false)
	if err != nil || info.Title != "Socket" {
		t.Fatalf("Open over the port endpoint: info=%+v err=%v", info, err)
	}
	snap, err := s.Snapshot(ctx, "", "")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	findRef(t, snap, `button "Go"`)
}
