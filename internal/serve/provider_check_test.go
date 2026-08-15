package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The endpoint in the fixture does not exist, so the probe fails — and that
// failure is the answer the row shows, carried as a finding rather than as a
// transport error the client has to guess at.
func TestCheckProviderReportsWhatTheEndpointSaid(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/providers/check", `{"name":"existing"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := readAllString(resp)
		t.Fatalf("POST /providers/check = %d: %s", resp.StatusCode, b)
	}
	var got providerCheck
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.OK {
		t.Fatalf("an unreachable endpoint reported ok: %+v", got)
	}
	if got.Error == "" {
		t.Fatal("a failed probe must carry the endpoint's own words")
	}
	if len(got.Models) != 0 {
		t.Fatalf("a failed probe reported models: %v", got.Models)
	}
}

func TestCheckProviderRefusesAnUnknownName(t *testing.T) {
	s := newProviderEditServer(t)
	s.AllowProviderEdit()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/providers/check", `{"name":"nobody"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("checking an unknown provider = %d, want 404", resp.StatusCode)
	}
}

// A check spends the machine's stored credential against a network endpoint, so
// it waits on the same grant every other provider edit does.
func TestCheckProviderWaitsOnTheGrant(t *testing.T) {
	s := newProviderEditServer(t)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := postProvider(t, srv.URL, "/providers/check", `{"name":"existing"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST /providers/check without the grant = %d, want 403", resp.StatusCode)
	}
}
