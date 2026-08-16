package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"reasonix/internal/billing"
	"reasonix/internal/config"
	"reasonix/internal/control"
)

const walletBody = `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"10.00","granted_balance":"0","topped_up_balance":"10.00"}]}`

// walletStub is a balance endpoint that counts what reaches it.
func walletStub(t *testing.T, hits *atomic.Int64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, walletBody)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func statusServer(t *testing.T, balanceURL string) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	active := filepath.Join(dir, "20260101-000000.000000000-status.jsonl")
	if err := os.WriteFile(active, []byte(`{"role":"user","content":"hi"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc, SessionDir: dir, SessionPath: active, Balance: billing.NewCache(nil, balanceURL, "")})
	srv := httptest.NewServer(New(ctrl, bc, config.ServeConfig{}).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func readStatus(t *testing.T, srv *httptest.Server) map[string]any {
	t.Helper()
	resp, err := http.Get(srv.URL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestStatusPollingHitsWalletOnce is the effect test for the balance cache: the
// desktop polls /status four times a second while a turn runs, and every one of
// those used to be a network round trip to the provider.
func TestStatusPollingHitsWalletOnce(t *testing.T) {
	var hits atomic.Int64
	wallet := walletStub(t, &hits)
	srv := statusServer(t, wallet.URL)

	// Five seconds of the desktop's 250ms poll.
	for range 20 {
		if _, ok := readStatus(t, srv)["balance"]; !ok {
			t.Fatal("status carried no balance")
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("wallet requests = %d, want 1", got)
	}
}

// TestStatusConcurrentPollsShareOneFetch proves panes opening at once do not
// each start their own request for the same wallet.
func TestStatusConcurrentPollsShareOneFetch(t *testing.T) {
	var hits atomic.Int64
	wallet := walletStub(t, &hits)
	srv := statusServer(t, wallet.URL)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { readStatus(t, srv) })
	}
	wg.Wait()
	if got := hits.Load(); got != 1 {
		t.Fatalf("wallet requests = %d, want 1", got)
	}
}

// TestStatusWithoutWalletNeverFetches proves a provider that declares no
// balance_url is never queried.
func TestStatusWithoutWalletNeverFetches(t *testing.T) {
	srv := statusServer(t, "")
	if _, ok := readStatus(t, srv)["balance"]; ok {
		t.Fatal("status carried a balance with no endpoint configured")
	}
}
