package computer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"reasonix/internal/visionimage"
)

// fakeHelperEnv makes the test binary answer as the native helper, so the
// protocol, the geometry and the failures run without a real application.
const fakeHelperEnv = "REASONIX_COMPUTER_FAKE_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(fakeHelperEnv) != "" {
		runFakeHelper(os.Stdin, os.Stdout)
		return
	}
	os.Exit(m.Run())
}

func fakeScreenshot() []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4000, 2000)))
	return buf.Bytes()
}

func runFakeHelper(in io.Reader, out io.Writer) {
	enc := json.NewEncoder(out)
	var lastClick map[string]any
	asked := 0
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		var req struct {
			ID     int64          `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil {
			continue
		}
		reply := map[string]any{"id": req.ID, "result": map[string]any{}}
		refuse := func(code, message string) {
			delete(reply, "result")
			reply["error"] = map[string]any{"code": code, "message": message}
		}
		switch req.Method {
		case "apps":
			reply["result"] = map[string]any{"apps": []any{
				map[string]any{"pid": 7, "bundle": "com.example.Notes", "name": "Notes", "windows": []any{}},
				map[string]any{"pid": 8, "bundle": "com.example.Locked", "name": "Locked", "windows": []any{}},
			}}
		case "snapshot":
			if req.Params["pid"] == float64(8) {
				refuse("computer.permission_missing", "accessibility")
			}
		case "request_permission":
			asked++
		case "screenshot":
			reply["result"] = map[string]any{
				"data": base64.StdEncoding.EncodeToString(fakeScreenshot()), "mime": "image/png",
				"bounds": map[string]any{"x": 100, "y": 50, "width": 2000, "height": 1000},
			}
		case "click":
			lastClick = req.Params
			reply["result"] = map[string]any{"role": "button"}
		case "press":
			refuse("computer.stale_ref", "a9 is gone")
		case "key":
			if req.Params["key"] == "Escape" {
				_ = enc.Encode(map[string]any{"event": "stop"})
				time.Sleep(20 * time.Millisecond)
			}
		case "_last_click":
			reply["result"] = lastClick
		case "_asked":
			reply["result"] = map[string]any{"asked": asked}
		case "_exit":
			os.Exit(0)
		}
		_ = enc.Encode(reply)
	}
}

func fakeSession(t *testing.T) *Session {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeHelperEnv, "1")
	return NewSession(NewHelper(exe))
}

func TestARefusedApplicationIsRefusedBeforeTheHelperIsAsked(t *testing.T) {
	s := NewSession(NewHelper("/nonexistent/helper"))
	for _, bundle := range []string{"com.apple.Terminal", " io.reasonix.studio ", "com.apple.systempreferences"} {
		if _, err := s.Act(context.Background(), bundle, []Step{{Action: "key", Key: "Enter"}}); CodeOf(err) != CodeAppRefused {
			t.Errorf("%q = %v, want %s", bundle, err, CodeAppRefused)
		}
	}
	if _, err := s.Apps(context.Background()); CodeOf(err) != CodeUnavailable {
		t.Fatalf("a helper that cannot start = %v, want %s", err, CodeUnavailable)
	}
}

func TestAClickAtAPointIsReadInTheLatestScreenshotsPixels(t *testing.T) {
	s := fakeSession(t)
	ctx := context.Background()
	x, y := 100.0, 40.0
	click := []Step{{Action: "click", X: &x, Y: &y}}
	if _, err := s.Act(ctx, "com.example.Notes", click); CodeOf(err) != CodeNeedsScreenshot {
		t.Fatalf("a click before any screenshot = %v, want %s", err, CodeNeedsScreenshot)
	}
	shot, app, err := s.Screenshot(ctx, "com.example.Notes")
	if err != nil || app.PID != 7 {
		t.Fatalf("Screenshot = %v for %+v", err, app)
	}
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(shot, "data:image/png;base64,"))
	fitted, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("the screenshot the model sees does not decode: %v", err)
	}
	if _, _, err := visionimage.Fit(raw, "image/png"); err != nil || fitted.Width >= 4000 {
		t.Fatalf("the screenshot was not fitted for a vision model: %dpx, %v", fitted.Width, err)
	}
	res, err := s.Act(ctx, "com.example.Notes", click)
	if err != nil || res.Notes[0] != "click the button at (100,40)" {
		t.Fatalf("click = %+v, %v", res, err)
	}
	var got struct{ X, Y float64 }
	if err := s.helper.Call(ctx, "_last_click", nil, &got); err != nil {
		t.Fatal(err)
	}
	scale := 2000 / float64(fitted.Width)
	if got.X != 100+x*scale || got.Y != 50+y*scale {
		t.Fatalf("the helper was sent (%v,%v), want (%v,%v) in screen points", got.X, got.Y, 100+x*scale, 50+y*scale)
	}
}

func TestAMissingPermissionIsRequestedOnceAndSaysWhatThePersonDoes(t *testing.T) {
	s := fakeSession(t)
	ctx := context.Background()
	for range 2 {
		_, err := s.Snapshot(ctx, "com.example.Locked")
		if CodeOf(err) != CodePermissionMissing || !strings.Contains(err.Error(), "Privacy & Security → Accessibility") {
			t.Fatalf("snapshot without accessibility = %v", err)
		}
	}
	var r struct{ Asked int }
	if err := s.helper.Call(ctx, "_asked", nil, &r); err != nil || r.Asked != 1 {
		t.Fatalf("the permission was requested %d times (%v), want once", r.Asked, err)
	}
}

func TestStepsStopAtTheFirstFailureAndWhenThePersonPressesEscape(t *testing.T) {
	s := fakeSession(t)
	ctx := context.Background()
	res, err := s.Act(ctx, "com.example.Notes", []Step{{Action: "key", Key: "Enter"}, {Action: "click", Ref: "a9"}, {Action: "key", Key: "Enter"}})
	if CodeOf(err) != CodeStaleRef || res.Done != 1 || res.FailedAt != 1 {
		t.Fatalf("a stale ref = %+v, %v", res, err)
	}
	res, err = s.Act(ctx, "com.example.Notes", []Step{{Action: "key", Key: "Escape"}, {Action: "key", Key: "Enter"}})
	if CodeOf(err) != CodeStopped || res.Done != 1 || res.FailedAt != 1 {
		t.Fatalf("Escape = %+v, %v", res, err)
	}
	if _, err := s.Act(ctx, "com.example.Notes", []Step{{Action: "drag"}}); CodeOf(err) != CodeBadStep {
		t.Fatalf("an unknown action = %v, want %s", err, CodeBadStep)
	}
	if _, err := s.Act(ctx, "com.example.Missing", nil); CodeOf(err) != CodeNoApp {
		t.Fatalf("an application that is not running = %v, want %s", err, CodeNoApp)
	}
}

func TestAHelperThatExitsFailsWhatWasPendingAndStartsAgain(t *testing.T) {
	s := fakeSession(t)
	ctx := context.Background()
	if err := s.helper.Call(ctx, "_exit", nil, nil); CodeOf(err) != CodeFailed {
		t.Fatalf("a call the helper died under = %v, want %s", err, CodeFailed)
	}
	if !(&Failure{Code: CodeFailed}).Is(&Failure{Code: CodeFailed, Detail: "other"}) {
		t.Fatal("failures with one code do not match")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		apps, err := s.Apps(ctx)
		if err == nil && len(apps) == 2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the helper did not start again: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
