package serve

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"reasonix/internal/config"
)

// writeWalletConfig points the default provider at a wallet endpoint the test
// owns, so the runtimes a hub builds really do query it.
func writeWalletConfig(t *testing.T, walletURL string) {
	t.Helper()
	t.Setenv("REASONIX_HOME", t.TempDir())
	cfgPath := config.UserConfigPath()
	if cfgPath == "" {
		t.Fatal("user config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`default_model = "default/shared-chat"

[[providers]]
name = "default"
kind = "openai"
base_url = "http://127.0.0.1:1/v1"
balance_url = %q
models = ["shared-chat"]
default = "shared-chat"
`, walletURL)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestHubPanesShareOneWalletRead is the effect test for the shared balance
// store: opening a conversation builds a whole runtime, so a per-runtime cache
// would be cold on exactly the read a session switch waits on.
func TestHubPanesShareOneWalletRead(t *testing.T) {
	var hits atomic.Int64
	wallet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, walletBody)
	}))
	defer wallet.Close()
	writeWalletConfig(t, wallet.URL)

	h := NewHub(HubOptions{})
	defer h.Shutdown()
	srv := httptest.NewServer(h.Handler())
	defer srv.Close()

	// Three panes, the way switching between conversations opens them.
	for range 3 {
		rt, err := h.Open(context.Background(), OpenRequest{Root: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		got := hubGet[map[string]any](t, srv, "/rt/"+rt.ID+"/status")
		if _, ok := got["balance"]; !ok {
			t.Fatalf("pane %s carried no balance", rt.ID)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("wallet requests = %d, want 1", got)
	}
}
