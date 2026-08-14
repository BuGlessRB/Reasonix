package main

import (
	"context"
	"sort"
	"strings"
	"time"

	"reasonix/desktop/internal/update"
	"reasonix/internal/config"
	"reasonix/internal/netclient"
)

// version is stamped by the build (-X main.version=…); an unstamped dev build
// says so rather than pretending to be a release.
var version = "dev"

// VersionEntry is one published release as the panel shows it.
type VersionEntry struct {
	Version     string `json:"version"`
	Tag         string `json:"tag"`
	PublishedAt string `json:"publishedAt"`
	Notes       string `json:"notes"`
	Current     bool   `json:"current"`
	Older       bool   `json:"older"`
}

// VersionHub is everything the version panel renders from. Err is carried
// alongside the data rather than replacing it: an unreachable catalog must not
// hide which version is running.
type VersionHub struct {
	Current  string         `json:"current"`
	Pinned   string         `json:"pinned"`
	StalePin bool           `json:"stalePin"`
	Latest   string         `json:"latest"`
	Newer    bool           `json:"newer"`
	Versions []VersionEntry `json:"versions"`
	Err      string         `json:"err,omitempty"`
}

// Versions reads the rollback catalog through the shared updater, so the shell
// holds no copy of what "newer", "pinned" or "latest" mean.
func (a *App) Versions() VersionHub {
	hub := VersionHub{Current: version}
	pinned := ""
	if cfg, err := config.Load(); err == nil && cfg != nil {
		pinned = cfg.DesktopPinnedVersion()
	}
	hub.Pinned = pinned
	client, err := netclient.NewHTTPClient(proxySpecForUpdates(), netclient.TransportOptions{})
	if err != nil {
		hub.Err, hub.Versions = err.Error(), versionRows(nil, version)
		return hub.nonNil()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	st, err := update.New(update.Options{Current: version, Pinned: pinned, HTTP: client}).Check(ctx)
	hub.Latest, hub.Newer, hub.StalePin = st.Latest, st.Newer, st.StalePin
	hub.Versions = versionRows(st.Entries, version)
	if err != nil {
		hub.Err = err.Error()
	}
	return hub.nonNil()
}

// versionRows merges the running build into the catalog, newest first. It is
// always present even when the catalog cannot be read or does not carry it (a
// local build, a release still publishing): the row is the panel's statement of
// what runs and its only handle for pinning, so it cannot depend on the network.
func versionRows(entries []update.IndexEntry, current string) []VersionEntry {
	rows := make([]VersionEntry, 0, len(entries)+1)
	seen := false
	for _, e := range entries {
		row := VersionEntry{Version: e.Version, Tag: e.Tag, PublishedAt: e.PublishedAt}
		if update.SameVersion(e.Version, current) {
			row.Current, seen = true, true
		} else {
			row.Older = e.IsOlderThan(current)
		}
		rows = append(rows, row)
	}
	if !seen && strings.TrimSpace(current) != "" {
		rows = append(rows, VersionEntry{Version: current, Current: true})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return update.CompareVersions(rows[i].Version, rows[j].Version) > 0
	})
	return rows
}

// nonNil keeps an empty catalog an empty list. A nil slice marshals to null,
// and a client that maps over it crashes its whole render — the same reason
// serve's session listing does this.
func (h VersionHub) nonNil() VersionHub {
	if h.Versions == nil {
		h.Versions = []VersionEntry{}
	}
	return h
}

// PinVersion holds this machine on a release, or releases the hold when version
// is empty. It is the half of a rollback that outlives the install: without it
// the updater would put the user back on the build they left.
func (a *App) PinVersion(pin string) error {
	cfg := config.LoadForEdit(config.UserConfigPath())
	if err := cfg.SetDesktopPinnedVersion(pin); err != nil {
		return err
	}
	return cfg.SaveTo(config.UserConfigPath())
}

func proxySpecForUpdates() netclient.ProxySpec {
	if cfg, err := config.Load(); err == nil && cfg != nil {
		return cfg.NetworkProxySpec()
	}
	return netclient.ProxySpec{Mode: netclient.ModeAuto}
}
