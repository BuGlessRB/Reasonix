package serve

import (
	"errors"
	"testing"
	"time"
)

func TestCloudPresenceTracksActiveControllersAndStableOrdinals(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	presence := newCloudPresence()
	presence.now = func() time.Time { return now }

	if got := presence.connect("browser-a", " Web   Studio "); got != 1 {
		t.Fatalf("first ordinal = %d, want 1", got)
	}
	now = now.Add(time.Minute)
	presence.touch("browser-a")
	if got := presence.connect("browser-b", "Web Studio"); got != 2 {
		t.Fatalf("second ordinal = %d, want 2", got)
	}

	views := presence.views()
	if len(views) != 2 || views[0].Name != "Web Studio" || views[0].Ordinal != 1 || !views[0].LastSeen.Equal(now) {
		t.Fatalf("views = %+v", views)
	}
	presence.disconnect("browser-a")
	views = presence.views()
	if len(views) != 1 || views[0].ID != "browser-b" {
		t.Fatalf("after disconnect = %+v", views)
	}
}

func TestCloudControllerCanBeRevokedFromDeviceShare(t *testing.T) {
	share := NewDeviceShare(nil)
	share.cloud.connect("browser-a", "Web Studio")
	called := ""
	share.setCloudDisconnect(func(id string) error {
		called = id
		share.cloud.disconnect(id)
		return nil
	})
	if !share.Revoke("browser-a") || called != "browser-a" {
		t.Fatalf("revoke = %q, want browser-a", called)
	}
	share.cloud.connect("browser-b", "Web Studio")
	share.setCloudDisconnect(func(string) error { return errors.New("relay unavailable") })
	if share.Revoke("browser-b") {
		t.Fatal("failed relay disconnect reported success")
	}
}

func TestHubCloudPresenceAppearsInShareStatus(t *testing.T) {
	share := NewDeviceShare(nil)
	hub := NewHub(HubOptions{Share: share})
	if got := hub.CloudControllerConnected("browser-a", "Web Studio"); got != 1 {
		t.Fatalf("ordinal = %d, want 1", got)
	}
	status := share.Status()
	if len(status.CloudDevices) != 1 || status.CloudDevices[0].ID != "browser-a" {
		t.Fatalf("cloud devices = %+v", status.CloudDevices)
	}
	hub.CloudControllerDisconnected("browser-a")
	if got := share.Status().CloudDevices; len(got) != 0 {
		t.Fatalf("cloud devices after disconnect = %+v", got)
	}
}
