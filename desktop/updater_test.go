package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"reasonix/desktop/internal/update"
	"reasonix/internal/repair"
)

func TestNormalizeVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"dev", "", false},
		{"", "", false},
		{"  ", "", false},
		{"1.2.3", "v1.2.3", true},
		{"v1.2.3", "v1.2.3", true},
		{"v1.2", "v1.2.0", true}, // semver.Canonical fills the patch
		{"garbage", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeVersion(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("normalizeVersion(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestValidateUpdaterRequestBindsChannelVersionAndID(t *testing.T) {
	tests := []struct {
		name    string
		request string
		channel string
		version string
		wantErr bool
	}{
		{name: "stable", request: "web-stable-1", channel: "stable", version: "v1.18.0"},
		{name: "legacy preview selects official", request: "web-preview-1", channel: "preview", version: "v1.18.0"},
		{name: "legacy preview rejects prerelease", request: "web-preview-2", channel: "preview", version: "v1.18.0-preview.1", wantErr: true},
		{name: "stable rejects preview version", request: "web-stable-2", channel: "stable", version: "v1.18.0-preview.1", wantErr: true},
		{name: "empty request", channel: "stable", version: "v1.18.0", wantErr: true},
		{name: "unsafe request", request: "web request", channel: "stable", version: "v1.18.0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, selected, version, err := validateUpdaterRequest(tt.request, tt.channel, tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateUpdaterRequest() error = %v, wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if request != tt.request || selected != "stable" || version != tt.version {
				t.Fatalf("validateUpdaterRequest() = (%q, %q, %q)", request, selected, version)
			}
		})
	}
}

func TestValidateAssetInstallLayout(t *testing.T) {
	if err := validateAssetInstallLayout(""); err != nil {
		t.Fatalf("empty layout must remain accepted for legacy assets: %v", err)
	}
	if err := validateAssetInstallLayout("versioned-v1"); err != nil {
		t.Fatalf("versioned-v1 must be accepted: %v", err)
	}
	if err := validateAssetInstallLayout("unknown-layout"); err == nil {
		t.Fatal("unknown install_layout must be rejected")
	}
}

func TestUpdaterWailsMethodContracts(t *testing.T) {
	appType := reflect.TypeFor[*App]()
	tests := []struct {
		name   string
		numIn  int
		numOut int
	}{
		{name: "ApplyUpdateRequest", numIn: 4, numOut: 1},
		{name: "CheckUpdate", numIn: 2, numOut: 2},
		{name: "OpenDownloadPage", numIn: 1, numOut: 0},
	}
	// Legacy Download/Install split bindings must stay deleted (v1.20+).
	for _, removed := range []string{"DownloadUpdate", "InstallUpdate", "DownloadUpdateRequest", "InstallUpdateRequest", "ApplyUpdate"} {
		if _, ok := appType.MethodByName(removed); ok {
			t.Fatalf("App.%s must be removed from the Wails surface", removed)
		}
	}
	for _, tt := range tests {
		method, ok := appType.MethodByName(tt.name)
		if !ok {
			t.Fatalf("App.%s is missing", tt.name)
		}
		if method.Type.NumIn() != tt.numIn || method.Type.NumOut() != tt.numOut {
			t.Fatalf(
				"App.%s signature = %v inputs/%v outputs, want %v/%v",
				tt.name,
				method.Type.NumIn(),
				method.Type.NumOut(),
				tt.numIn,
				tt.numOut,
			)
		}
	}
}

func TestUpdaterNativeOperationsFailFastWhileBusy(t *testing.T) {
	app := NewApp()
	finishFirst, err := app.beginUpdaterOperation("first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.beginUpdaterOperation("second"); !errors.Is(err, errUpdateInProgress) {
		t.Fatalf("second updater operation error = %v, want errUpdateInProgress", err)
	}
	finishFirst()
	finishSecond, err := app.beginUpdaterOperation("second")
	if err != nil {
		t.Fatalf("operation did not become available after release: %v", err)
	}
	finishSecond()
}

func TestUpdaterReconcilesPendingUpdateBeforeInstallModeDispatch(t *testing.T) {
	originalExists := pendingUpdateExistsForInstall
	originalArchive := archiveSupersededPendingUpdateForInstall
	originalReconcile := reconcilePendingUpdateForInstall
	t.Cleanup(func() {
		pendingUpdateExistsForInstall = originalExists
		archiveSupersededPendingUpdateForInstall = originalArchive
		reconcilePendingUpdateForInstall = originalReconcile
	})

	called := false
	pendingUpdateExistsForInstall = func() bool { return true }
	archiveSupersededPendingUpdateForInstall = func() (bool, error) { return false, nil }
	reconcilePendingUpdateForInstall = func(runningVersion string) (repair.PendingUpdateReconcileResult, error) {
		called = true
		if runningVersion != version {
			t.Fatalf("running version = %q, want %q", runningVersion, version)
		}
		return repair.PendingUpdateReconcileResult{Pending: true, Cleared: true}, nil
	}
	if err := (&App{}).reconcilePendingUpdateForRequest("install-1", "stable", "v1.18.0-preview.65", 42); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("pending update reconciliation was skipped")
	}
}

func TestUpdaterArchivesSupersededUpdateBeforeReconciliation(t *testing.T) {
	originalExists := pendingUpdateExistsForInstall
	originalArchive := archiveSupersededPendingUpdateForInstall
	originalReconcile := reconcilePendingUpdateForInstall
	t.Cleanup(func() {
		pendingUpdateExistsForInstall = originalExists
		archiveSupersededPendingUpdateForInstall = originalArchive
		reconcilePendingUpdateForInstall = originalReconcile
	})

	archived := false
	pendingUpdateExistsForInstall = func() bool { return true }
	archiveSupersededPendingUpdateForInstall = func() (bool, error) {
		archived = true
		return true, nil
	}
	reconcilePendingUpdateForInstall = func(string) (repair.PendingUpdateReconcileResult, error) {
		if !archived {
			t.Fatal("reconciliation ran before superseded update archival")
		}
		return repair.PendingUpdateReconcileResult{}, nil
	}
	if err := (&App{}).reconcilePendingUpdateForRequest("install-1", "stable", "v1.20.0", 42); err != nil {
		t.Fatal(err)
	}
}

func TestUpdaterReconcilesBeforeDownloading(t *testing.T) {
	originalExists := pendingUpdateExistsForInstall
	originalArchive := archiveSupersededPendingUpdateForInstall
	originalReconcile := reconcilePendingUpdateForInstall
	t.Cleanup(func() {
		pendingUpdateExistsForInstall = originalExists
		archiveSupersededPendingUpdateForInstall = originalArchive
		reconcilePendingUpdateForInstall = originalReconcile
	})

	pendingUpdateExistsForInstall = func() bool { return true }
	archiveSupersededPendingUpdateForInstall = func() (bool, error) { return false, nil }
	reconcilePendingUpdateForInstall = func(string) (repair.PendingUpdateReconcileResult, error) {
		return repair.PendingUpdateReconcileResult{Pending: true}, errors.New("blocked before download")
	}
	err := (&App{}).ApplyUpdateRequest("stable", "v1.20.0", "preflight-recovery")
	if err == nil || !strings.Contains(err.Error(), "blocked before download") {
		t.Fatalf("pre-download recovery error=%v", err)
	}
}

func TestUpdaterBlocksInstallWhilePreviousReleaseAwaitsHealth(t *testing.T) {
	originalExists := pendingUpdateExistsForInstall
	originalArchive := archiveSupersededPendingUpdateForInstall
	originalReconcile := reconcilePendingUpdateForInstall
	t.Cleanup(func() {
		pendingUpdateExistsForInstall = originalExists
		archiveSupersededPendingUpdateForInstall = originalArchive
		reconcilePendingUpdateForInstall = originalReconcile
	})

	pendingUpdateExistsForInstall = func() bool { return true }
	archiveSupersededPendingUpdateForInstall = func() (bool, error) {
		return false, errors.New("not a superseded flat-layout transaction")
	}
	reconcilePendingUpdateForInstall = func(string) (repair.PendingUpdateReconcileResult, error) {
		return repair.PendingUpdateReconcileResult{Pending: true, AwaitingHealth: true}, repair.ErrPendingUpdateAwaitingHealth
	}
	err := (&App{}).reconcilePendingUpdateForRequest("install-1", "stable", "v1.18.0-preview.65", 42)
	if err == nil || !strings.Contains(err.Error(), "startup health check") {
		t.Fatalf("health-check recovery error = %v", err)
	}
}

func TestExpectedUpdateVersionRejectsAdvancedPointer(t *testing.T) {
	if err := ensureExpectedUpdateVersion("preview", "v1.18.0-preview.1", "v1.18.0-preview.2"); err == nil {
		t.Fatal("advanced pointer unexpectedly matched the checked version")
	}
	if err := ensureExpectedUpdateVersion("stable", "v1.18.0", "v1.18.0"); err != nil {
		t.Fatalf("identical pointer rejected: %v", err)
	}
}

func TestUpdateSiblingNamesCoverEveryReplacedEntryPoint(t *testing.T) {
	windows := strings.Join(updateSiblingNames("windows"), "\x00")
	for _, want := range []string{"reasonix-guard.exe", "reasonix-launcher.exe", "reasonix-update-helper.exe", "reasonix-cli.exe", "Reasonix.exe"} {
		if !strings.Contains(windows, want) {
			t.Errorf("Windows release unit omits %q: %q", want, windows)
		}
	}
	if strings.Contains(windows, "reasonix.exe") {
		t.Fatalf("Windows release unit reintroduces the case-only CLI/launcher collision: %q", windows)
	}
	if got := updateSiblingNames("linux"); len(got) != 2 || got[0] != "reasonix-guard" || got[1] != "reasonix" {
		t.Fatalf("Linux release unit = %q", got)
	}
	if got := updateSiblingNames("darwin"); got != nil {
		t.Fatalf("macOS app-bundle update must not list file siblings: %q", got)
	}
}

func TestEvaluate(t *testing.T) {
	mk := func(version string) *update.Manifest {
		return &update.Manifest{
			Version:   version,
			Notes:     "notes",
			Platforms: map[string]update.Asset{update.CurrentPlatform(): {Size: 999}},
		}
	}
	portable := installProfile{
		Mode:          installModePortable,
		CanSelfUpdate: runtime.GOOS != "darwin",
		ArtifactKind:  update.KindTarball,
	}
	if runtime.GOOS == "darwin" {
		portable.Mode = installModeManual
		portable.ManualReason = manualUpdateReason()
	}

	if got := evaluateWithProfile("v1.0.0", mk("v1.1.0"), portable); !got.Available {
		t.Error("v1.0.0 -> v1.1.0 should be available")
	}
	if got := evaluateWithProfile("v1.1.0", mk("v1.1.0"), portable); got.Available {
		t.Error("same version should not be available")
	}
	if got := evaluateWithProfile("v1.2.0", mk("v1.1.0"), portable); got.Available {
		t.Error("newer-than-manifest should not be available")
	}
	// A dev build must never auto-prompt, even against a real release.
	if got := evaluateWithProfile("dev", mk("v1.1.0"), portable); got.Available {
		t.Error("dev build should not prompt to update")
	}
	// An invalid manifest version is a check error, not an update.
	got := evaluateWithProfile("v1.0.0", mk("not-a-version"), portable)
	if got.Available || got.Err == "" {
		t.Errorf("invalid manifest version: got %+v", got)
	}
	// Metadata carries through.
	full := evaluateWithProfile("v1.0.0", mk("v1.1.0"), portable)
	if full.Latest != "v1.1.0" || full.Notes != "notes" || full.AssetSize != 999 {
		t.Errorf("metadata not carried: %+v", full)
	}
	if full.CanSelfUpdate != (runtime.GOOS != "darwin") {
		t.Errorf("CanSelfUpdate = %v on %s", full.CanSelfUpdate, runtime.GOOS)
	}
	if full.InstallMode == "" {
		t.Error("InstallMode should be set")
	}
}

func TestEvaluateDebSelectsNativePackage(t *testing.T) {
	if runtime.GOOS == "darwin" && !canSelfUpdate() {
		// evaluateWithProfile applies the macOS signed-build gate; synthetic deb
		// profiles are only meaningful on Linux (or a notarized macOS build).
		t.Skip("deb install mode is a Linux packaging path")
	}
	m := &update.Manifest{
		Version: "v2.0.0",
		Platforms: map[string]update.Asset{
			update.CurrentPlatform(): {URL: "https://example/tarball", Size: 100, SHA256: "aa"},
		},
		NativePackages: map[string]update.Asset{
			update.CurrentPlatform(): {URL: "https://example/pkg.deb", Size: 200, SHA256: "bb"},
		},
	}
	deb := installProfile{
		Mode:          installModeDeb,
		CanSelfUpdate: true,
		RequiresElev:  true,
		ArtifactKind:  update.KindDeb,
	}
	got := evaluateWithProfile("v1.0.0", m, deb)
	if !got.Available || got.AssetSize != 200 {
		t.Fatalf("deb evaluate should use native package size: %+v", got)
	}
	if !got.RequiresElevation || got.InstallMode != installModeDeb {
		t.Fatalf("deb flags missing: %+v", got)
	}
	// Without native_packages, deb profile becomes manual.
	m2 := &update.Manifest{
		Version:   "v2.0.0",
		Platforms: map[string]update.Asset{update.CurrentPlatform(): {Size: 100}},
	}
	adjusted := profileForManifest(deb, m2)
	got = evaluateWithProfile("v1.0.0", m2, adjusted)
	if got.CanSelfUpdate || got.InstallMode != installModeManual {
		t.Fatalf("missing native package should force manual: %+v (profile=%+v)", got, adjusted)
	}
}

func TestManualUpdateRequiredErrorPreservesReason(t *testing.T) {
	err := manualUpdateRequiredError(installProfile{ManualReason: "system update helper is unavailable"})
	if !errors.Is(err, errUpdateManualRequired) {
		t.Fatalf("error = %v, want manual-update sentinel", err)
	}
	if !strings.Contains(err.Error(), "system update helper is unavailable") {
		t.Fatalf("error = %q, want profile reason", err)
	}
}

func TestLegacyChannelsSelectOfficialPointers(t *testing.T) {
	stable := manifestEndpoints("stable")
	preview := manifestEndpoints("preview")
	want := []string{
		r2Base + "/latest/latest.json",
		releaseGatewayBase + "/stable/latest.json",
		githubManifestFallback,
	}
	if !reflect.DeepEqual(stable, want) || !reflect.DeepEqual(preview, want) {
		t.Fatalf("manifest endpoints: stable=%q preview=%q want=%q", stable, preview, want)
	}
	if got := downloadPage("preview"); got != "https://reasonix.io/?download=desktop#start" {
		t.Errorf("legacy preview download page = %q", got)
	}
	if got := manifestDownloadPage("preview", "https://reasonix.io/?channel=preview&download=desktop#start"); got != "https://reasonix.io/?download=desktop#start" {
		t.Errorf("manifest official page = %q", got)
	}
	if got := manifestDownloadPage("preview", "https://example.com/releases"); got != "https://example.com/releases" {
		t.Errorf("external manifest download page = %q, want unchanged", got)
	}
	for _, unsafe := range []string{
		"javascript:alert(1)",
		"http://reasonix.io/#start",
		"https://user@reasonix.io/#start",
	} {
		if got := manifestDownloadPage("preview", unsafe); got != downloadPage("stable") {
			t.Errorf("unsafe manifest page %q = %q, want official fallback", unsafe, got)
		}
	}
}

func TestManifestChannelValidation(t *testing.T) {
	tests := []struct {
		name      string
		channel   string
		version   string
		wantError bool
	}{
		{name: "stable release", channel: "stable", version: "v1.17.21"},
		{name: "legacy preview selects official", channel: "preview", version: "v1.17.21"},
		{name: "legacy canary selects official", channel: "canary", version: "v1.17.21"},
		{name: "legacy preview rejects prerelease", channel: "preview", version: "v1.18.0-preview.7", wantError: true},
		{name: "legacy canary rejects prerelease", channel: "canary", version: "v1.17.21-canary.56", wantError: true},
		{name: "Stable rejects Preview", channel: "stable", version: "v1.18.0-preview.7", wantError: true},
		{name: "Stable requires v prefix", channel: "stable", version: "1.17.21", wantError: true},
		{name: "Stable rejects build metadata", channel: "stable", version: "v1.17.21+build.1", wantError: true},
		{name: "Stable rejects prerelease", channel: "stable", version: "v1.17.21-rc.1", wantError: true},
		{name: "invalid version", channel: "preview", version: "dev", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateManifestChannel(tt.channel, &update.Manifest{Version: tt.version})
			if (err != nil) != tt.wantError {
				t.Fatalf("validateManifestChannel(%q, %q) error = %v, wantError=%v", tt.channel, tt.version, err, tt.wantError)
			}
		})
	}
}

func validDesktopManifest(t *testing.T, selected, manifestVersion string) update.Manifest {
	t.Helper()
	tag := desktopReleaseTag(selected, manifestVersion)
	manifest := update.Manifest{
		Version:        manifestVersion,
		DownloadPage:   manifestDownloadPageURL,
		Platforms:      map[string]update.Asset{},
		NativePackages: map[string]update.Asset{},
		Downloads:      map[string]update.Asset{},
	}
	requiredAssets := append([]requiredDesktopAsset(nil), requiredDesktopUpdaterAssets...)
	requiredAssets = append(requiredAssets, requiredDesktopDownloadAssets...)
	for _, required := range requiredAssets {
		assetURL := fmt.Sprintf("%s/%s/%s", r2Base, tag, required.filename)
		asset := update.Asset{
			URL:    assetURL,
			Sig:    assetURL + ".minisig",
			Size:   1024,
			SHA256: strings.Repeat("a", 64),
		}
		switch required.group {
		case "platforms":
			manifest.Platforms[required.key] = asset
		case "native_packages":
			manifest.NativePackages[required.key] = asset
		case "downloads":
			manifest.Downloads[required.key] = asset
		}
	}
	return manifest
}

func TestDesktopManifestValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*update.Manifest)
	}{
		{
			name: "missing required platform asset",
			mutate: func(m *update.Manifest) {
				delete(m.Platforms, "darwin-arm64")
			},
		},
		{
			name: "wrong filename",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.URL = strings.Replace(asset.URL, "Reasonix-", "Other-", 1)
				asset.Sig = asset.URL + ".minisig"
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "HTTP asset URL",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.URL = strings.Replace(asset.URL, "https://", "http://", 1)
				asset.Sig = asset.URL + ".minisig"
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "asset URL userinfo",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.URL = strings.Replace(asset.URL, "https://", "https://user@", 1)
				asset.Sig = asset.URL + ".minisig"
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "wrong asset host",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.URL = strings.Replace(asset.URL, "dl.reasonix.io", "example.com", 1)
				asset.Sig = asset.URL + ".minisig"
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "wrong release tag",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.URL = strings.Replace(asset.URL, desktopReleaseTag("stable", m.Version), "desktop-v9.9.9", 1)
				asset.Sig = asset.URL + ".minisig"
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "signature is not exact URL suffix",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.Sig = asset.URL + ".sig"
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "zero size",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.Size = 0
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "negative size",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.Size = -1
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "size above release maximum",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.Size = maxDesktopReleaseAssetSize + 1
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "uppercase SHA",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.SHA256 = strings.Repeat("A", 64)
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "short SHA",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.SHA256 = strings.Repeat("a", 63)
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "nonhex SHA",
			mutate: func(m *update.Manifest) {
				asset := m.Platforms["darwin-arm64"]
				asset.SHA256 = strings.Repeat("g", 64)
				m.Platforms["darwin-arm64"] = asset
			},
		},
		{
			name: "missing download page",
			mutate: func(m *update.Manifest) {
				m.DownloadPage = ""
			},
		},
		{
			name: "wrong download page",
			mutate: func(m *update.Manifest) {
				m.DownloadPage = "https://reasonix.io/?channel=stable&download=desktop#start"
			},
		},
	}

	if err := validateDesktopManifest("stable", ptr(validDesktopManifest(t, "stable", "v1.18.0"))); err != nil {
		t.Fatalf("valid Stable manifest: %v", err)
	}
	if err := validateDesktopManifest("preview", ptr(validDesktopManifest(t, "stable", "v1.19.0"))); err != nil {
		t.Fatalf("legacy Preview selection did not accept official manifest: %v", err)
	}
	t.Run("legacy manifests remain upgradeable", func(t *testing.T) {
		stable := validDesktopManifest(t, "stable", "v1.17.21")
		stable.Downloads = nil
		if err := validateDesktopManifest("stable", &stable); err != nil {
			t.Fatalf("legacy Stable manifest: %v", err)
		}
	})
	t.Run("empty downloads is not a legacy manifest", func(t *testing.T) {
		manifest := validDesktopManifest(t, "stable", "v1.17.21")
		manifest.Downloads = map[string]update.Asset{}
		if err := validateDesktopManifest("stable", &manifest); err == nil {
			t.Fatal("manifest with empty downloads bypassed the new-format asset requirements")
		}
	})
	t.Run("official manifest rejects legacy rolling asset base", func(t *testing.T) {
		manifest := validDesktopManifest(t, "stable", "v1.19.0")
		immutableBase := r2Base + "/desktop-v1.19.0/"
		rollingBase := r2Base + "/desktop-preview/"
		for key, asset := range manifest.Platforms {
			asset.URL = strings.Replace(asset.URL, immutableBase, rollingBase, 1)
			asset.Sig = asset.URL + ".minisig"
			manifest.Platforms[key] = asset
		}
		for key, asset := range manifest.NativePackages {
			asset.URL = strings.Replace(asset.URL, immutableBase, rollingBase, 1)
			asset.Sig = asset.URL + ".minisig"
			manifest.NativePackages[key] = asset
		}
		for key, asset := range manifest.Downloads {
			asset.URL = strings.Replace(asset.URL, immutableBase, rollingBase, 1)
			asset.Sig = asset.URL + ".minisig"
			manifest.Downloads[key] = asset
		}
		if err := validateDesktopManifest("stable", &manifest); err == nil {
			t.Fatal("official manifest accepted mutable rolling assets")
		}
	})
	t.Run("unified GitHub release base", func(t *testing.T) {
		manifest := validDesktopManifest(t, "stable", "v1.19.0")
		oldBase := r2Base + "/desktop-v1.19.0/"
		newBase := "https://github.com/esengine/DeepSeek-Reasonix/releases/download/v1.19.0/"
		for key, asset := range manifest.Platforms {
			asset.URL = strings.Replace(asset.URL, oldBase, newBase, 1)
			asset.Sig = asset.URL + ".minisig"
			manifest.Platforms[key] = asset
		}
		for key, asset := range manifest.NativePackages {
			asset.URL = strings.Replace(asset.URL, oldBase, newBase, 1)
			asset.Sig = asset.URL + ".minisig"
			manifest.NativePackages[key] = asset
		}
		for key, asset := range manifest.Downloads {
			asset.URL = strings.Replace(asset.URL, oldBase, newBase, 1)
			asset.Sig = asset.URL + ".minisig"
			manifest.Downloads[key] = asset
		}
		if err := validateDesktopManifest("stable", &manifest); err != nil {
			t.Fatalf("unified GitHub release manifest: %v", err)
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := validDesktopManifest(t, "stable", "v1.18.0")
			tt.mutate(&manifest)
			if err := validateDesktopManifest("stable", &manifest); err == nil {
				t.Fatal("validateDesktopManifest accepted malformed manifest")
			}
		})
	}

	t.Run("invalid native package", func(t *testing.T) {
		manifest := validDesktopManifest(t, "stable", "v1.18.0")
		native := manifest.NativePackages["linux-amd64"]
		native.Sig = native.URL + ".sig"
		manifest.NativePackages["linux-amd64"] = native
		if err := validateDesktopManifest("stable", &manifest); err == nil {
			t.Fatal("validateDesktopManifest accepted malformed native package")
		}
	})

	t.Run("mixed official bases", func(t *testing.T) {
		manifest := validDesktopManifest(t, "stable", "v1.18.0")
		asset := manifest.Platforms["darwin-arm64"]
		asset.URL = strings.Replace(
			asset.URL,
			r2Base+"/desktop-v1.18.0/",
			"https://github.com/esengine/DeepSeek-Reasonix/releases/download/desktop-v1.18.0/",
			1,
		)
		asset.Sig = asset.URL + ".minisig"
		manifest.Platforms["darwin-arm64"] = asset
		if err := validateDesktopManifest("stable", &manifest); err == nil {
			t.Fatal("validateDesktopManifest accepted mixed R2 and GitHub asset bases")
		}
	})
}

func ptr[T any](value T) *T {
	return &value
}

func TestFetchManifestSkipsPrereleaseForLegacyPreviewSelection(t *testing.T) {
	var calls []string
	client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.String())
		version := "v1.18.0-preview.7"
		if strings.Contains(req.URL.Path, "/stable/") {
			version = "v1.18.0"
		}
		manifest := validDesktopManifest(t, "stable", version)
		body, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	manifest, err := fetchManifest(context.Background(), client, nil, "preview")
	if err != nil {
		t.Fatalf("fetchManifest: %v", err)
	}
	if manifest.Version != "v1.18.0" {
		t.Fatalf("version = %q, want official fallback manifest", manifest.Version)
	}
	if len(calls) != 2 || !strings.Contains(calls[0], "/latest/") || !strings.Contains(calls[1], "/stable/") {
		t.Fatalf("endpoint calls = %q, want official latest then gateway fallback", calls)
	}
}

func TestFetchManifestSkipsMalformedSuccessfulResponse(t *testing.T) {
	var calls []string
	client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.String())
		manifest := validDesktopManifest(t, "stable", "v1.18.0")
		if strings.Contains(req.URL.Path, "/latest/") {
			delete(manifest.Platforms, update.CurrentPlatform())
		}
		body, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})}

	manifest, err := fetchManifest(context.Background(), client, nil, "preview")
	if err != nil {
		t.Fatalf("fetchManifest: %v", err)
	}
	if manifest.Version != "v1.18.0" {
		t.Fatalf("version = %q, want valid fallback manifest", manifest.Version)
	}
	if len(calls) != 2 || !strings.Contains(calls[0], "/latest/") || !strings.Contains(calls[1], "/stable/") {
		t.Fatalf("endpoint calls = %q, want malformed 200 to fall through", calls)
	}
}

func TestValidateUpdateRedirect(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		wantError bool
	}{
		{name: "Reasonix first-party redirect", target: "https://dl.reasonix.io/file"},
		{name: "GitHub redirect", target: "https://github.com/file"},
		{name: "GitHub HTTPS asset redirect", target: "https://release-assets.githubusercontent.com/file"},
		{name: "HTTPS downgrade", target: "http://release-assets.githubusercontent.com/file", wantError: true},
		{name: "userinfo", target: "https://user@release-assets.githubusercontent.com/file", wantError: true},
		{name: "missing hostname", target: "https:///file", wantError: true},
		{name: "arbitrary HTTPS host", target: "https://example.com/file", wantError: true},
		{name: "Reasonix suffix spoof", target: "https://dl.reasonix.io.evil.invalid/file", wantError: true},
		{name: "GitHub suffix spoof", target: "https://release-assets.githubusercontent.com.evil.invalid/file", wantError: true},
		{name: "explicit port", target: "https://dl.reasonix.io:443/file", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tt.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = validateUpdateRedirect(req, nil)
			if (err != nil) != tt.wantError {
				t.Fatalf("validateUpdateRedirect(%q) error = %v, wantError=%v", tt.target, err, tt.wantError)
			}
		})
	}
	t.Run("redirect limit", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "https://release-assets.githubusercontent.com/file", nil)
		if err != nil {
			t.Fatal(err)
		}
		via := make([]*http.Request, 10)
		if err := validateUpdateRedirect(req, via); err == nil {
			t.Fatal("validateUpdateRedirect accepted more than 10 redirects")
		}
	})
}

func withUpdateCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	restore := update.CacheDirFn
	update.CacheDirFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { update.CacheDirFn = restore })
	return dir
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// The cache lives in the update package; what the shell owns is pointing it at
// the user's cache directory and reporting the hit as UpdateInfo.Downloaded.
func TestSaveCachedUpdateMarksEvaluateDownloaded(t *testing.T) {
	withUpdateCacheDir(t)
	oldChannel := channel
	channel = "stable"
	t.Cleanup(func() { channel = oldChannel })

	data := []byte("verified artifact")
	asset := update.Asset{
		URL:    "https://dl.reasonix.io/studio-v9.9.9/Reasonix-linux-amd64.tar.gz",
		Size:   int64(len(data)),
		SHA256: sha256Hex(data),
	}
	manifest := &update.Manifest{
		Version:   "v9.9.9",
		Platforms: map[string]update.Asset{update.CurrentPlatform(): asset},
	}
	portable := installProfile{Mode: installModePortable, CanSelfUpdate: true, ArtifactKind: update.KindTarball}
	if got := evaluateWithProfile("v1.0.0", manifest, portable); got.Downloaded {
		t.Fatal("fresh cache should not report a downloaded update")
	}
	cache, err := updateCache()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Save("v9.9.9", asset, data, update.KindTarball, nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := evaluateWithProfile("v1.0.0", manifest, portable); !got.Downloaded {
		t.Fatalf("evaluate did not detect cached update: %+v", got)
	}
}

func TestLegacyChannelAliasDoesNotPermitDowngrade(t *testing.T) {
	oldChannel := channel
	channel = "preview"
	t.Cleanup(func() { channel = oldChannel })

	m := &update.Manifest{
		Version: "v1.6.0",
		Platforms: map[string]update.Asset{
			update.CurrentPlatform(): {Size: 100},
		},
	}
	got := evaluateWithProfileForChannel(
		"v1.7.0-preview.12",
		"stable",
		m,
		installProfile{Mode: installModePortable, CanSelfUpdate: true},
	)
	if got.Available {
		t.Fatalf("legacy Preview alias permitted an official downgrade: %+v", got)
	}
	if got.Channel != "stable" {
		t.Fatalf("channel = %q, want stable", got.Channel)
	}
}

func TestProfileForManifestDebWithoutHelperBecomesManual(t *testing.T) {
	// profileForManifest checks linuxDebHelperReady(); on non-linux it is always
	// false, so a synthetic deb profile without native assets becomes manual.
	base := installProfile{Mode: installModeDeb, CanSelfUpdate: true, RequiresElev: true, ArtifactKind: update.KindDeb}
	m := &update.Manifest{Version: "v1.0.0"}
	got := profileForManifest(base, m)
	if got.Mode != installModeManual || got.CanSelfUpdate {
		t.Fatalf("expected manual without native package: %+v", got)
	}
}

func TestExtractBinary(t *testing.T) {
	want := []byte("#!/bin/sh\necho reasonix\n")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := map[string][]byte{"README": []byte("ignore me"), "reasonix-desktop": want}
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()

	got, err := extractBinary(buf.Bytes(), "reasonix-desktop")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("extracted %q, want %q", got, want)
	}
	if _, err := extractBinary(buf.Bytes(), "missing"); err == nil {
		t.Error("missing entry should error")
	}
}

func TestApplyLinuxHoldsReleaseUnitLockDuringReplace(t *testing.T) {
	dir := robustTempDir(t)
	t.Setenv("REASONIX_HOME", robustTempDir(t))
	exe := filepath.Join(dir, "reasonix-desktop")
	releasePaths := releaseUnitPathsFor(dir, "linux")
	for _, path := range releasePaths {
		if err := os.WriteFile(path, []byte("old"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := repair.PrepareFileUpdate("v1", "v2", exe, releasePaths[1:]...)
	if err != nil {
		t.Fatal(err)
	}
	originalPath := currentExecutablePathForLinux
	originalApply := applyLinuxReleaseUnit
	currentExecutablePathForLinux = func() string { return exe }
	entered := make(chan struct{})
	releaseReplace := make(chan struct{})
	applyLinuxReleaseUnit = func(
		tx *repair.UpdateTransaction,
		exe string,
		bin, guard, cli []byte,
	) ([]repair.FileUpdateInstallReceipt, error) {
		close(entered)
		<-releaseReplace
		return originalApply(tx, exe, bin, guard, cli)
	}
	t.Cleanup(func() {
		currentExecutablePathForLinux = originalPath
		applyLinuxReleaseUnit = originalApply
	})

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"reasonix-desktop", "reasonix-guard", "reasonix"} {
		body := []byte(name)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	applyDone := make(chan error, 1)
	go func() { applyDone <- applyLinux(buf.Bytes(), prepared) }()
	select {
	case <-entered:
	case err := <-applyDone:
		t.Fatalf("applyLinux failed before replacement: %v", err)
	}

	lockDone := make(chan error, 1)
	go func() {
		unlock, err := repair.LockRepairMutations(releasePaths...)
		if err == nil {
			unlock()
		}
		lockDone <- err
	}()
	select {
	case err := <-lockDone:
		close(releaseReplace)
		t.Fatalf("competing updater lock acquired during Linux replacement: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(releaseReplace)
	if err := <-applyDone; err != nil {
		t.Fatalf("applyLinux: %v", err)
	}
	if err := <-lockDone; err != nil {
		t.Fatalf("competing lock after replacement: %v", err)
	}
	if _, ok := repair.ReadUpdateApplyFailure(); ok {
		t.Fatal("successful Linux release-unit publish left an interruption marker")
	}
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
