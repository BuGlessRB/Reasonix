package billing

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const cacheWalletBody = `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"10.00","granted_balance":"0","topped_up_balance":"10.00"}]}`

func cacheStub(t *testing.T, hits *atomic.Int64, fail *atomic.Bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		if fail != nil && fail.Load() {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, cacheWalletBody)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// expire ages the cache past its freshness window without waiting for it.
func expire(c *Cache) {
	c.mu.Lock()
	c.state.tried = c.state.tried.Add(-freshFor - time.Second)
	c.mu.Unlock()
}

func TestCacheAnswersFromMemoryWhileFresh(t *testing.T) {
	var hits atomic.Int64
	srv := cacheStub(t, &hits, nil)
	c := NewCache(nil, srv.URL, "")
	for range 10 {
		b, err := c.Get(context.Background())
		if err != nil || b == nil {
			t.Fatalf("Get = %v, %v", b, err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("fetches = %d, want 1", got)
	}
}

func TestCacheRefetchesAfterFreshness(t *testing.T) {
	var hits atomic.Int64
	srv := cacheStub(t, &hits, nil)
	c := NewCache(nil, srv.URL, "")
	if _, err := c.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	expire(c)
	if _, err := c.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("fetches = %d, want 2", got)
	}
}

// A wallet that answered a moment ago beats a blank readout: the endpoint being
// briefly unreachable is not the account being empty.
func TestCacheStandsInForAFailingWallet(t *testing.T) {
	var hits atomic.Int64
	var fail atomic.Bool
	srv := cacheStub(t, &hits, &fail)
	c := NewCache(nil, srv.URL, "")
	if _, err := c.Get(context.Background()); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	expire(c)
	b, err := c.Get(context.Background())
	if err != nil || b == nil {
		t.Fatalf("a failing wallet blanked the readout: %v, %v", b, err)
	}

	// Past the stand-in window the error is the answer, not a stale balance.
	c.mu.Lock()
	c.state.got = c.state.got.Add(-standsIn - time.Second)
	c.mu.Unlock()
	expire(c)
	if b, err := c.Get(context.Background()); err == nil || b != nil {
		t.Fatalf("stale balance outlived its window: %v, %v", b, err)
	}
}

// A first fetch that fails has nothing to stand in for, so it reports.
func TestCacheReportsFirstFailure(t *testing.T) {
	var hits atomic.Int64
	var fail atomic.Bool
	fail.Store(true)
	srv := cacheStub(t, &hits, &fail)
	c := NewCache(nil, srv.URL, "")
	if b, err := c.Get(context.Background()); err == nil || b != nil {
		t.Fatalf("Get = %v, %v; want an error", b, err)
	}
	// Even a failure is throttled — a broken endpoint must not be retried on
	// every poll.
	if _, err := c.Get(context.Background()); err == nil {
		t.Fatal("second Get lost the error")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("fetches = %d, want 1", got)
	}
}

func TestCacheWithoutURLNeverFetches(t *testing.T) {
	c := NewCache(nil, "  ", "")
	b, err := c.Get(context.Background())
	if b != nil || err != nil {
		t.Fatalf("Get = %v, %v; want (nil, nil)", b, err)
	}
}
