package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reasonix/internal/testenv"
)

type stubUpdateHost struct{ acknowledged int }

func (s *stubUpdateHost) AcknowledgeLaunchHealth() error { s.acknowledged++; return nil }

// Sharing an update engine with a desktop application must not hand a
// networked kernel the power to replace one. Owning the application is what
// makes the routes exist, so a hub without an owner has none to refuse.
func TestUpdateRoutesExistOnlyWhereSomethingOwnsTheApplication(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	bare := httptest.NewServer(NewHub(HubOptions{}).Handler())
	defer bare.Close()

	resp, err := http.Post(bare.URL+"/update/health", "application/json", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		t.Fatalf("POST /update/health answered %d with no application behind it", resp.StatusCode)
	}
}

// And the other direction, because a gate nothing passes is not a gate: the
// acknowledgement reaches the owner, carrying nothing of its own.
func TestAnOwnedApplicationAcknowledgesItsOwnLaunch(t *testing.T) {
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	host := &stubUpdateHost{}
	owned := httptest.NewServer(NewHub(HubOptions{Update: host}).Handler())
	defer owned.Close()

	resp, err := http.Post(owned.URL+"/update/health", "application/json", strings.NewReader(`{"transaction":"somebody else's"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /update/health = %d, want 204", resp.StatusCode)
	}
	if host.acknowledged != 1 {
		t.Fatalf("acknowledged %d times, want 1", host.acknowledged)
	}
}
