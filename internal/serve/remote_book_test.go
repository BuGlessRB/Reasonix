package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/config"
)

func bookServer(t *testing.T, attacher RemoteAttacher) *httptest.Server {
	t.Helper()
	t.Setenv("REASONIX_HOME", t.TempDir())
	front := httptest.NewServer(NewHub(HubOptions{Remote: attacher}).Handler())
	t.Cleanup(front.Close)
	return front
}

func bookPost(t *testing.T, front *httptest.Server, path, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(front.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// A machine is reachable from every project on this computer, so its entry
// belongs to the user and not to whichever folder was open when it was added.
func TestSavingAHostWritesTheUserBook(t *testing.T) {
	front := bookServer(t, &stubAttacher{})
	resp := bookPost(t, front, "/remotes",
		`{"name":"gpu-box","host":"10.0.0.4","user":"ada","port":2222,"workspace":"/srv/training"}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := cfg.RemoteHost("gpu-box")
	if !ok {
		t.Fatal("the host never reached the book")
	}
	if entry.Host != "10.0.0.4" || entry.User != "ada" || entry.Port != 2222 {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Workspace != "/srv/training" {
		t.Fatalf("workspace = %q", entry.Workspace)
	}
}

// An entry layering ssh_config has its address written next door; one that does
// not has nowhere else to get it, and a row with neither dials nothing.
func TestAHostNeedsAnAddressUnlessSSHConfigHasIt(t *testing.T) {
	front := bookServer(t, &stubAttacher{})
	resp := bookPost(t, front, "/remotes", `{"name":"gpu-box"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "remote.host_required" {
		t.Fatalf("code = %q", body.Code)
	}

	// The same row with ssh_config layered under it is complete: the alias is
	// the address, which is the whole point of importing one.
	if got := bookPost(t, front, "/remotes", `{"name":"gpu-box","useSSHConfig":true}`).StatusCode; got != http.StatusNoContent {
		t.Fatalf("ssh_config entry rejected: %d", got)
	}
	cfg, _ := config.Load()
	entry, _ := cfg.RemoteHost("gpu-box")
	if entry.Host != "gpu-box" || !entry.UseSSHConfig {
		t.Fatalf("entry = %+v, want the alias standing in for the address", entry)
	}
}

// Removing the row under a live pane would leave a connection nothing accounts
// for: the pane keeps driving a kernel the book no longer knows about.
func TestRemovingAHostIsRefusedWhileItDrivesAPane(t *testing.T) {
	t.Setenv("REASONIX_HOME", t.TempDir())
	rk := fakeRemoteKernel(t)
	h := NewHub(HubOptions{Remote: &stubAttacher{}})
	if _, err := h.OpenRemote(RemoteEndpoint{
		Host: "gpu-box", Workspace: "/srv/training", Addr: rk.Listener.Addr().String(), Token: "t",
	}, nil); err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(h.Handler())
	defer front.Close()

	resp := bookPost(t, front, "/remotes/remove", `{"name":"gpu-box"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	var body struct {
		Code   string         `json:"code"`
		Params map[string]any `json:"params"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "remote.has_open_panes" {
		t.Fatalf("code = %q", body.Code)
	}
	if n, _ := body.Params["n"].(float64); n != 1 {
		t.Fatalf("params.n = %v, want the pane count", body.Params["n"])
	}
}

// Offering an alias the book already holds would invite a duplicate row for one
// machine, each with its own connection.
func TestCandidatesLeaveOutWhatTheBookAlreadyHas(t *testing.T) {
	front := bookServer(t, &stubAttacher{candidates: []string{"gpu-box", "builder", "attic"}})
	if got := bookPost(t, front, "/remotes", `{"name":"builder","host":"build.internal"}`).StatusCode; got != http.StatusNoContent {
		t.Fatalf("save failed: %d", got)
	}
	resp, err := http.Get(front.URL + "/remotes/candidates")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0] != "gpu-box" || out[1] != "attic" {
		t.Fatalf("candidates = %v, want the two not in the book", out)
	}
}

// The page has no control for forwards — they are set from the CLI — so an edit
// made here must not be how they disappear.
func TestEditingAHostKeepsForwardsThePageCannotSee(t *testing.T) {
	front := bookServer(t, &stubAttacher{})
	if got := bookPost(t, front, "/remotes", `{"name":"gpu-box","host":"10.0.0.4"}`).StatusCode; got != http.StatusNoContent {
		t.Fatalf("save failed: %d", got)
	}
	if err := config.EditUserConfigWithCredentials(func(c *config.Config) ([]config.CredentialChange, error) {
		entry, _ := c.RemoteHost("gpu-box")
		entry.Forwards = []config.RemoteForwardEntry{{Type: "local", Bind: "127.0.0.1:8080", Target: "127.0.0.1:80"}}
		return nil, c.UpsertRemoteHost(entry)
	}); err != nil {
		t.Fatal(err)
	}

	if got := bookPost(t, front, "/remotes", `{"name":"gpu-box","host":"10.0.0.9"}`).StatusCode; got != http.StatusNoContent {
		t.Fatalf("edit failed: %d", got)
	}
	cfg, _ := config.Load()
	entry, _ := cfg.RemoteHost("gpu-box")
	if entry.Host != "10.0.0.9" {
		t.Fatalf("the edit did not land: %+v", entry)
	}
	if len(entry.Forwards) != 1 || entry.Forwards[0].Bind != "127.0.0.1:8080" {
		t.Fatalf("the edit dropped the forwards: %+v", entry.Forwards)
	}
}

// The far machine's own workspace list, read through a pane already open on
// it. Without one there is no kernel over there to ask, and saying so beats an
// empty list that reads as "this machine has nothing".
func TestRemoteTreeIsReadThroughAnOpenPane(t *testing.T) {
	writeOpenableConfig(t)
	far := NewHub(HubOptions{})
	defer far.Shutdown()
	farSide := httptest.NewServer(far.Handler())
	defer farSide.Close()

	near := NewHub(HubOptions{Remote: &stubAttacher{
		attach: func(host, workspace string) (RemoteEndpoint, func(), error) {
			return RemoteEndpoint{
				Host: host, Workspace: workspace,
				Addr: farSide.Listener.Addr().String(), Token: "t",
			}, func() {}, nil
		},
	}})
	nearSide := httptest.NewServer(near.Handler())
	defer nearSide.Close()

	resp, err := http.Get(nearSide.URL + "/remotes/gpu-box/tree")
	if err != nil {
		t.Fatal(err)
	}
	body := struct {
		Code string `json:"code"`
	}{}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict || body.Code != "remote.not_connected" {
		t.Fatalf("unconnected host = %d/%q, want 409/remote.not_connected", resp.StatusCode, body.Code)
	}

	workspace := t.TempDir()
	open, err := http.Post(nearSide.URL+"/remotes/open", "application/json", openRemoteBody(t, "gpu-box", workspace))
	if err != nil {
		t.Fatal(err)
	}
	defer open.Body.Close()
	if open.StatusCode != http.StatusOK {
		t.Fatalf("open remote = %d", open.StatusCode)
	}

	resp, err = http.Get(nearSide.URL + "/remotes/gpu-box/tree")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tree = %d, want 200", resp.StatusCode)
	}
	var tree []struct {
		Root string `json:"root"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ws := range tree {
		if ws.Root == workspace {
			found = true
		}
	}
	if !found {
		t.Fatalf("the far machine's tree does not list the workspace a pane opened: %+v", tree)
	}
}
