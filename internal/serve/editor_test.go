package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/config"
	"reasonix/internal/control"
)

func editorServer(t *testing.T, granted bool) *Server {
	t.Helper()
	ctrl := control.New(control.Options{})
	t.Cleanup(ctrl.Close)
	s := New(ctrl, NewBroadcaster(), config.ServeConfig{})
	if granted {
		s.AllowEditorOpen()
	}
	return s
}

// The editor launches on the machine running the kernel, so a server reached
// over the network must refuse rather than open a window nobody is sitting at.
// The grant is the gate, and it says which class of refusal this is.
func TestEditorOpenRefusesWithoutAHostGrant(t *testing.T) {
	s := editorServer(t, false)
	rec := httptest.NewRecorder()
	s.openInEditor(rec, httptest.NewRequest(http.MethodPost, "/workspace/editor", nil))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("refusal is not an envelope: %s", rec.Body.String())
	}
	if body.Code != codeEditorNoWindow {
		t.Errorf("code = %q, want %q", body.Code, codeEditorNoWindow)
	}
}

// A machine with no editor is not a failure to launch one: the two ask the
// person for different things, so they carry different codes.
func TestEditorOpenSeparatesMissingFromFailedToStart(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("ProgramFiles", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	s := editorServer(t, true)
	rec := httptest.NewRecorder()
	s.openInEditor(rec, httptest.NewRequest(http.MethodPost, "/workspace/editor", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), codeEditorMissing) {
		t.Errorf("refusal = %s, want %s", rec.Body.String(), codeEditorMissing)
	}
}
