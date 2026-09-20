package main

import (
	"os"
	"path/filepath"
	"testing"

	"reasonix/internal/config"
)

func TestHistoricalDiscoveryIgnoresEmptyCanonicalDirectories(t *testing.T) {
	isolateDesktopUserDirs(t)
	root := config.SessionStoreDir()
	for _, id := range []string{"empty-read-probe", "damaged-history"} {
		if err := os.MkdirAll(filepath.Join(root, id), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "damaged-history", "manifest.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	app := newHistoricalLifecycleApp(t)
	status, err := app.ListHistoricalSessions()
	if err != nil || len(status.Items) != 1 || status.Items[0].Title != "damaged-history" {
		t.Fatalf("empty probes should be hidden but damaged artifacts preserved: %+v %v", status, err)
	}
}

func TestHistoricalRestoredSourceClearsOnCanonicalActivation(t *testing.T) {
	tab := &WorkspaceTab{HistoricalSource: &SessionSourceRef{HostID: localDesktopHostID, Path: "/fixture/old.jsonl"}}
	setTabSessionIdentity(tab, remoteSessionIDRoutePrefix+"canonical-target")
	if tab.HistoricalSource != nil {
		t.Fatal("pending source leaked into the next session")
	}
	if tab.SessionID != "canonical-target" {
		t.Fatal("test must activate a canonical identity")
	}
}
