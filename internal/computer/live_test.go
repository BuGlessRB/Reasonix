package computer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"reasonix/internal/testenv"
)

const targetBundle = "io.reasonix.test.computer-target"

// liveSession drives the real helper named by REASONIX_LIVE_COMPUTER. It needs
// macOS, swiftc, and accessibility and screen recording granted to whatever
// runs the test, so ordinary runs skip.
func liveSession(t *testing.T) *Session {
	t.Helper()
	path := os.Getenv("REASONIX_LIVE_COMPUTER")
	if path == "" || runtime.GOOS != "darwin" {
		t.Skip("set REASONIX_LIVE_COMPUTER to a built computer-use helper to run live tests")
	}
	return NewSession(NewHelper(path))
}

// launchTarget builds the test application into a bundle and opens it, so it
// runs with an identity like any other application.
func launchTarget(t *testing.T) string {
	t.Helper()
	dir := testenv.TempDir(t)
	macos := filepath.Join(dir, "Target.app", "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>` + targetBundle + `</string>
<key>CFBundleExecutable</key><string>target</string>
<key>CFBundleName</key><string>Computer Target</string>
<key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(dir, "Target.app", "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("swiftc", "-O", "testdata/target.swift", "-o", filepath.Join(macos, "target"))
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build target: %v\n%s", err, out)
	}
	log := filepath.Join(dir, "target.log")
	if out, err := exec.Command("open", "-g", "-n", filepath.Join(dir, "Target.app"), "--args", log).CombinedOutput(); err != nil {
		t.Fatalf("open target: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("pkill", "-f", filepath.Join(macos, "target")).Run() })
	waitLog(t, log, "ready")
	return log
}

func waitLog(t *testing.T, path, needle string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if body, _ := os.ReadFile(path); strings.Contains(string(body), needle) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	body, _ := os.ReadFile(path)
	t.Fatalf("the target never logged %q:\n%s", needle, body)
}

func lineRef(t *testing.T, lines []string, needle string) string {
	t.Helper()
	for _, l := range lines {
		if strings.Contains(l, needle) {
			if m := regexp.MustCompile(`\[(a\d+)\]`).FindStringSubmatch(l); m != nil {
				return m[1]
			}
		}
	}
	t.Fatalf("no ref on a line containing %q:\n%s", needle, strings.Join(lines, "\n"))
	return ""
}

func TestLiveOperatesAnApplicationWithoutItsPointer(t *testing.T) {
	s := liveSession(t)
	log := launchTarget(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	snap, err := s.Snapshot(ctx, targetBundle)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	t.Logf("snapshot:\n%s", strings.Join(snap.Lines, "\n"))
	field := lineRef(t, snap.Lines, `textField "Probe field"`)
	button := lineRef(t, snap.Lines, `button "Probe button"`)

	res, err := s.Act(ctx, targetBundle, []Step{
		{Action: "set_value", Ref: field, Text: "from-ax"},
		{Action: "click", Ref: button},
		{Action: "focus", Ref: field},
		{Action: "type", Text: " 李雷"},
	})
	if err != nil {
		t.Fatalf("Act: %v (done %d)", err, res.Done)
	}
	waitLog(t, log, "button pressed")
	waitLog(t, log, "from-ax 李雷")

	shot, app, err := s.Screenshot(ctx, targetBundle)
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if !strings.HasPrefix(shot, "data:image/") || app.Bundle != targetBundle {
		t.Fatalf("screenshot = %.40q for %+v", shot, app)
	}
	// The button's centre in the window, in points, scaled into the image.
	s.mu.Lock()
	geometry := s.shots[targetBundle]
	s.mu.Unlock()
	x, y := 345/geometry.scale, (geometry.bounds.Height-162)/geometry.scale
	if _, err := s.Act(ctx, targetBundle, []Step{{Action: "click", X: &x, Y: &y}}); err != nil {
		t.Fatalf("click at a screenshot point: %v", err)
	}
	cx, cy := 100/geometry.scale, (geometry.bounds.Height-70)/geometry.scale
	if _, err := s.Act(ctx, targetBundle, []Step{{Action: "click", X: &cx, Y: &cy}}); CodeOf(err) != CodeNoAction {
		t.Fatalf("clicking a view with no accessibility action = %v, want %s", err, CodeNoAction)
	}
	if _, err := s.Snapshot(ctx, "com.apple.Terminal"); CodeOf(err) != CodeAppRefused {
		t.Fatalf("a terminal = %v, want %s", err, CodeAppRefused)
	}
}

// The three a person does without thinking and an agent could not: open an
// element's own context menu, bring something into view, and wait.
func TestLiveContextMenuScrollAndWait(t *testing.T) {
	s := liveSession(t)
	log := launchTarget(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	snap, err := s.Snapshot(ctx, targetBundle)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	t.Logf("snapshot:\n%s", strings.Join(snap.Lines, "\n"))
	menu := lineRef(t, snap.Lines, `"Probe menu area"`)
	res, err := s.Act(ctx, targetBundle, []Step{
		{Action: "right_click", Ref: menu},
		{Action: "wait", Ms: 400},
	})
	if err != nil {
		t.Fatalf("right_click: %v (%v)", err, res.Notes)
	}
	waitLog(t, log, "menu opened")

	// Where the scroll area sits is what says the element was brought into
	// view: the application that offers pages rather than a reveal never hears
	// about the element at all.
	position := func() string {
		now, err := s.Snapshot(ctx, targetBundle)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		for _, line := range now.Lines {
			if strings.Contains(line, "scrollBar") {
				return line
			}
		}
		return ""
	}
	before := position()
	deep := lineRef(t, snap.Lines, `"Deep row"`)
	res, err = s.Act(ctx, targetBundle, []Step{{Action: "scroll", Ref: deep}})
	if err != nil || len(res.Notes) == 0 || !strings.Contains(res.Notes[0], "into view") {
		t.Fatalf("scroll to a ref: %v %v", err, res.Notes)
	}
	if after := position(); after == before {
		t.Fatalf("the scroll area did not move: %q", after)
	}

	if _, err := s.Act(ctx, targetBundle, []Step{{Action: "scroll", Amount: -3}}); err != nil {
		t.Fatalf("scroll by lines: %v", err)
	}
	if _, err := s.Act(ctx, targetBundle, []Step{{Action: "right_click"}}); CodeOf(err) != CodeBadStep {
		t.Fatalf("a right_click with no ref = %v, want %s", err, CodeBadStep)
	}
}
