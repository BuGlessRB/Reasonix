package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"reasonix/desktop/internal/update"
	"reasonix/internal/config"
	"reasonix/internal/installlayout"
	"reasonix/internal/netclient"
	"reasonix/internal/repair"
)

// updater.go is the transport-free core of the desktop auto-updater: manifest
// fetch, version comparison, signed download, and per-platform apply/relaunch. It
// has no Wails dependency so the logic is unit-tested directly; updater_app.go is
// the thin Wails binding that wires these into App methods and progress events.

// Manifest endpoints — R2 CDN first (fast, especially in CN), then the crash
// worker release gateway, then GitHub as the stable channel's last resort. The
// selected update channel picks the rolling pointer; it is user-configurable and
// independent from the build channel embedded for diagnostics/backcompat. The
// gateway still avoids GitHub's repository-wide /releases/latest shortcut so the
// app is not coupled to GitHub's homepage badge semantics.
const (
	r2Base                     = "https://dl.reasonix.io"
	releaseGatewayBase         = "https://crash.reasonix.io/v1/desktop/releases"
	downloadPageURL            = "https://reasonix.io/#start"
	manifestDownloadPageURL    = "https://reasonix.io/?download=desktop#start"
	httpTimeout                = 15 * time.Second
	manifestEndpointTimeout    = 5 * time.Second
	maxDesktopReleaseAssetSize = int64(1 << 30)
	maxDesktopManifestSize     = int64(1 << 20)
	maxDesktopSignatureSize    = int64(64 << 10)
)

var fetchAttemptTimeout = 5 * time.Second

var (
	stableDesktopVersionRE = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	sha256RE               = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// githubManifestFallback is the stable channel's last-resort manifest source.
// dl.reasonix.io and crash.reasonix.io share one Cloudflare zone, so bot
// protection that 403s a user's egress IP takes out both first-party endpoints
// at once (#6005); GitHub is separate infrastructure. Stable desktop releases
// own the repo-wide latest badge and publish latest.json directly, while
// The unified official Release carries the desktop manifest as a final fallback
// when both first-party endpoints are unavailable.
const githubManifestFallback = "https://github.com/esengine/DeepSeek-Reasonix/releases/latest/download/latest.json"

func normalizeUpdateChannel(ch string) string {
	return config.NormalizeDesktopUpdateChannel(ch)
}

func configuredUpdateChannel() string {
	cfg, err := config.Load()
	if err != nil {
		return "stable"
	}
	return cfg.DesktopUpdateChannel()
}

func targetUpdateChannel(selected string) string {
	_ = selected
	return configuredUpdateChannel()
}

func runningUpdateChannel() string {
	return normalizeUpdateChannel(channel)
}

// manifestEndpoints returns the manifest URLs for the selected update channel,
// in the order fetchManifest tries them.
func manifestEndpoints(selected string) []string {
	_ = selected
	return []string{
		r2Base + "/latest/latest.json",
		releaseGatewayBase + "/stable/latest.json",
		githubManifestFallback,
	}
}

// updaterUserAgent identifies updater traffic. Go's default Go-http-client UA
// is exactly what edge bot protection scores worst (#6005); a descriptive UA
// lets the release edge allowlist updater requests and makes them attributable
// in server logs.
func updaterUserAgent(selected string) string {
	return fmt.Sprintf("Reasonix-Updater/%s (%s/%s; build=%s; update=%s)", version, runtime.GOOS, runtime.GOARCH, channel, normalizeUpdateChannel(selected))
}

// downloadPage is the human-facing releases page shown when self-update is
// unavailable (macOS) or the manifest omits its own link.
func downloadPage(selected string) string {
	_ = selected
	u, _ := url.Parse(downloadPageURL)
	query := u.Query()
	query.Set("download", "desktop")
	query.Del("channel")
	u.RawQuery = query.Encode()
	return u.String()
}

func manifestDownloadPage(selected, manifestPage string) string {
	manifestPage = strings.TrimSpace(manifestPage)
	if manifestPage == "" {
		return downloadPage(selected)
	}
	u, err := url.Parse(manifestPage)
	if err != nil ||
		u.Scheme != "https" ||
		u.Hostname() == "" ||
		u.User != nil {
		return downloadPage(selected)
	}
	host := strings.ToLower(u.Hostname())
	if host != "reasonix.io" && !strings.HasSuffix(host, ".reasonix.io") {
		return u.String()
	}
	query := u.Query()
	query.Set("download", "desktop")
	query.Del("channel")
	u.RawQuery = query.Encode()
	u.Fragment = "start"
	return u.String()
}

// UpdateInfo is the CheckUpdate result that drives the frontend's update banner.
type UpdateInfo struct {
	Available         bool   `json:"available"`
	Current           string `json:"current"`
	Latest            string `json:"latest"`
	Notes             string `json:"notes"`
	Channel           string `json:"channel"`
	CanSelfUpdate     bool   `json:"canSelfUpdate"` // win/linux true; macOS true only for signed/notarized builds
	ManualOnly        bool   `json:"manualOnly,omitempty"`
	ManualReason      string `json:"manualReason,omitempty"`
	InstallMode       string `json:"installMode"`                 // portable | deb | manual
	RequiresElevation bool   `json:"requiresElevation,omitempty"` // deb/Polkit path
	Downloaded        bool   `json:"downloaded"`
	DownloadURL       string `json:"downloadUrl"`   // human-facing releases page (macOS path / fallback link)
	AssetSize         int64  `json:"assetSize"`     // running platform's artifact size, for the progress bar
	Err               string `json:"err,omitempty"` // set when the check itself failed (both endpoints down)
}

// UpdateDownloadResult is returned after an artifact has been downloaded,
// verified, and stored in the local updater cache.
type UpdateDownloadResult struct {
	RequestID string `json:"requestId"`
	Version   string `json:"version"`
	Channel   string `json:"channel"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
}

// updateProgress is the payload of the "updater:progress" Wails event emitted
// throughout DownloadUpdate / InstallUpdate.
type updateProgress struct {
	RequestID string `json:"requestId"`
	Version   string `json:"version"`
	Channel   string `json:"channel"`
	Phase     string `json:"phase"` // downloading | verifying | downloaded | authorizing | recovering | installing | done | error
	Received  int64  `json:"received"`
	Total     int64  `json:"total"`
	Err       string `json:"err,omitempty"`
}

func httpClient() (*http.Client, error) { return newHTTPClient(false) }

// httpClientIPv4 pins the dialer to IPv4 — the download fallback when the default
// (often IPv6-first) route to Cloudflare keeps resetting mid-transfer.
func httpClientIPv4() (*http.Client, error) { return newHTTPClient(true) }

func newHTTPClient(forceIPv4 bool) (*http.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	c, err := netclient.NewHTTPClient(cfg.NetworkProxySpec(), netclient.TransportOptions{ForceIPv4: forceIPv4})
	if err != nil {
		return nil, err
	}
	c.CheckRedirect = validateUpdateRedirect
	return c, nil
}

func validateUpdateRedirect(req *http.Request, via []*http.Request) error {
	return update.TrustedRedirect(req, via)
}

// canSelfUpdate reports whether in-place update is possible. Windows and Linux
// can replace the verified artifact directly; macOS requires an explicitly
// signed/notarized build flag so local or ad-hoc builds stay manual.
func canSelfUpdate() bool {
	return runtime.GOOS != "darwin" || macSelfUpdateAllowed()
}

func manualUpdateReason() string {
	if runtime.GOOS == "darwin" && !macSelfUpdateAllowed() {
		return "macOS automatic updates require a Developer ID signed and notarized build"
	}
	return ""
}

// normalizeVersion canonicalizes a version to semver "vX.Y.Z". It reports ok=false
// for the un-injected "dev" build (and anything not valid semver), so a dev build
// never prompts to update.
func normalizeVersion(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" {
		return "", false
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return "", false
	}
	return semver.Canonical(v), true
}

// fetchManifest pulls latest.json from each endpoint in order until one both
// responds, decodes, and matches an official release. Every endpoint's
// failure is kept — a user staring at a gateway 403 (#6005) needs to see that
// the R2 pointer failed too, not just whichever endpoint happened to die last.
func fetchManifest(ctx context.Context, c, fallback *http.Client, selected string) (*update.Manifest, error) {
	var errs []error
	selected = normalizeUpdateChannel(selected)
	for _, url := range manifestEndpoints(selected) {
		endpointCtx, cancel := context.WithTimeout(ctx, manifestEndpointTimeout)
		b, err := fetchManifestBytes(endpointCtx, c, fallback, selected, url)
		cancel()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var m update.Manifest
		if err := json.Unmarshal(b, &m); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", url, err))
			continue
		}
		if err := validateDesktopManifest(selected, &m); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", url, err))
			continue
		}
		return &m, nil
	}
	return nil, fmt.Errorf("update: fetch manifest: %w", errors.Join(errs...))
}

// fetchManifestBytes gives the default and IPv4 transports separate halves of
// the endpoint budget. A stalled IPv6 dial must not consume the whole timeout
// before the IPv4 fallback gets a chance to run (#6713).
func fetchManifestBytes(ctx context.Context, c, fallback *http.Client, selected, url string) ([]byte, error) {
	attemptTimeout := manifestEndpointTimeout / 2
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	data, err := transportFor(c, nil, selected).FetchOnce(attemptCtx, c, url, maxDesktopManifestSize)
	cancel()
	if err == nil || !update.Transient(err) || fallback == nil {
		return data, err
	}
	attemptCtx, cancel = context.WithTimeout(ctx, attemptTimeout)
	fallbackData, fallbackErr := transportFor(fallback, nil, selected).FetchOnce(attemptCtx, fallback, url, maxDesktopManifestSize)
	cancel()
	if fallbackErr == nil {
		return fallbackData, nil
	}
	return nil, errors.Join(err, fallbackErr)
}

// evaluateForChannel compares the running version against the selected channel's
// manifest and builds the frontend-facing result. I/O is limited to install-profile
// detection and cache probes so tests can inject a fixed profile below.
func evaluateForChannel(current, selected string, m *update.Manifest) UpdateInfo {
	return evaluateWithProfileForChannel(current, selected, m, profileForManifest(detectInstallProfile(), m))
}

func evaluateWithProfile(current string, m *update.Manifest, profile installProfile) UpdateInfo {
	return evaluateWithProfileForChannel(current, runningUpdateChannel(), m, profile)
}

// evaluateWithProfileForChannel is the pure comparison core once the install
// profile and selected update channel are known.
func evaluateWithProfileForChannel(current, selected string, m *update.Manifest, profile installProfile) UpdateInfo {
	selected = normalizeUpdateChannel(selected)
	page := manifestDownloadPage(selected, m.DownloadPage)
	info := UpdateInfo{
		Current:           current,
		Latest:            m.Version,
		Notes:             m.Notes,
		Channel:           selected,
		CanSelfUpdate:     profile.CanSelfUpdate,
		ManualOnly:        !profile.CanSelfUpdate,
		ManualReason:      profile.ManualReason,
		InstallMode:       profile.Mode,
		RequiresElevation: profile.RequiresElev,
		DownloadURL:       page,
	}
	// Preserve the pre-existing macOS gate when profile detection would otherwise
	// claim portable self-update on an unsigned build.
	if runtime.GOOS == "darwin" && !canSelfUpdate() {
		info.CanSelfUpdate = false
		info.ManualOnly = true
		info.RequiresElevation = false
		info.InstallMode = installModeManual
		if info.ManualReason == "" {
			info.ManualReason = manualUpdateReason()
		}
	}
	cur, okCur := normalizeVersion(current)
	latest, okLatest := normalizeVersion(m.Version)
	if !okLatest {
		info.Err = "manifest has no valid version"
		return info
	}
	// A dev/invalid running version never auto-prompts. Within a channel, only a
	// newer semver is an update. Across channels, a different target latest is an
	// explicit channel switch, so allow installing stable over a newer preview.
	if okCur {
		if selected != runningUpdateChannel() {
			info.Available = latest != cur
		} else if semver.Compare(latest, cur) > 0 {
			info.Available = true
		}
	}
	if a, kind, ok := selectUpdateAsset(m, profile); ok {
		info.AssetSize = a.Size
		info.Downloaded = cachedUpdateMatches(m.Version, a, kind)
	} else if a, ok := m.Asset(); ok {
		// Manual installs (or a missing native package) still surface the portable
		// artifact size so the UI can show how large the download is on the page.
		info.AssetSize = a.Size
	}
	return info
}

func updateCacheDir() (string, error) { return update.CacheDir() }

// updateCache is the one verified artifact this build has downloaded, wherever
// the user's cache directory turned out to be.
func updateCache() (update.Cache, error) {
	dir, err := updateCacheDir()
	if err != nil {
		return update.Cache{}, err
	}
	return update.Cache{Dir: dir}, nil
}

// updaterFor binds this build's transports, cache and artifact kind to the
// shared updater. The rolling pointer still finds what is newest (the catalog
// is not the desktop's discovery path), but everything after that — download,
// verify, cache — is the code a rollback runs.
func updaterFor(profile installProfile) (*update.Updater, error) {
	dir, err := updateCacheDir()
	if err != nil {
		return nil, err
	}
	c, err := httpClient()
	if err != nil {
		return nil, err
	}
	v4, _ := httpClientIPv4() // best-effort; nil just means retries reuse c
	pinned := ""
	if cfg, err := config.Load(); err == nil && cfg != nil {
		pinned = cfg.DesktopPinnedVersion()
	}
	return update.New(update.Options{
		Current:        version,
		Pinned:         pinned,
		HTTP:           c,
		Fallback:       v4,
		CacheDir:       dir,
		Kind:           profile.ArtifactKind,
		UserAgent:      updaterUserAgent(runningUpdateChannel()),
		AttemptTimeout: fetchAttemptTimeout,
	}), nil
}

func cachedUpdateMatches(version string, asset update.Asset, kind string) bool {
	c, err := updateCache()
	if err != nil {
		return false
	}
	return c.Holds(version, asset, kind)
}

// transportFor binds this build's user agent to the shared transport.
func transportFor(c, fallback *http.Client, selected string) update.Transport {
	return update.Transport{
		Client:         c,
		Fallback:       fallback,
		UserAgent:      updaterUserAgent(selected),
		AttemptTimeout: fetchAttemptTimeout,
	}
}

// extractBinary pulls a single named regular file out of a .tar.gz blob.
func extractBinary(targz []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(targz))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && (h.Name == name || strings.HasSuffix(h.Name, "/"+name)) {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("update: %q not found in archive", name)
}

// applyLinux replaces the running binary with the one inside the downloaded
// tar.gz; the caller relaunches afterwards.
func applyLinux(targz []byte, prepared *repair.UpdateTransaction) error {
	release, err := update.ExtractReleaseUnit(targz, desktopLine())
	if err != nil {
		return err
	}
	bin := release["reasonix-desktop"]
	guard := release["reasonix-guard"]
	cli := release["reasonix"]
	exe := currentExecutablePathForLinux()
	if exe == "" {
		return fmt.Errorf("update: current executable path is unavailable")
	}
	releasePaths := releaseUnitPathsFor(filepath.Dir(exe), "linux")
	if prepared == nil {
		return fmt.Errorf("update: prepared transaction is unavailable")
	}
	claimed, releaseClaim, err := repair.ClaimPendingFileUpdateExact(
		prepared.ToVersion,
		prepared.CreatedAt,
		repair.UpdateTransactionID(prepared),
		exe,
		releasePaths,
		2*time.Minute,
	)
	if err != nil {
		return fmt.Errorf("update: claim prepared transaction: %w", err)
	}
	defer releaseClaim()
	if err := repair.MarkUpdateApplyFailedExact(claimed, "Linux update publish did not complete"); err != nil {
		return fmt.Errorf("update: record recovery intent: %w", err)
	}
	receipts, err := applyLinuxReleaseUnit(claimed, exe, bin, guard, cli)
	if err != nil {
		return err
	}
	if _, err := repair.RecordClaimedFileUpdateInstalled(claimed, receipts...); err != nil {
		return fmt.Errorf("update: record installed release unit: %w", err)
	}
	// pending-update.json remains immutable; the transaction-unique sidecar now
	// binds every installed member. A crash before marker cleanup is safe:
	// startup correlates the exact transaction and rolls the release unit back.
	_ = repair.ClearUpdateApplyFailureExact(claimed)
	return nil
}

var currentExecutablePathForLinux = currentExecutablePath

var applyLinuxReleaseUnit = func(
	claimed *repair.UpdateTransaction,
	exe string,
	bin, guard, cli []byte,
) ([]repair.FileUpdateInstallReceipt, error) {
	receipts := make([]repair.FileUpdateInstallReceipt, 0, 3)
	receipt, err := repair.PublishClaimedFileUpdateMemberExact(claimed, filepath.Join(filepath.Dir(exe), "reasonix"), cli, 0o700)
	if err != nil {
		return receipts, fmt.Errorf("update CLI sidecar: %w", err)
	}
	receipts = append(receipts, receipt)
	receipt, err = repair.PublishClaimedFileUpdateMemberExact(claimed, filepath.Join(filepath.Dir(exe), "reasonix-guard"), guard, 0o700)
	if err != nil {
		return receipts, fmt.Errorf("update Guard: %w", err)
	}
	receipts = append(receipts, receipt)
	receipt, err = repair.PublishClaimedFileUpdateMemberExact(claimed, exe, bin, 0o700)
	if err != nil {
		return receipts, fmt.Errorf("update desktop: %w", err)
	}
	receipts = append(receipts, receipt)
	return receipts, nil
}

func applyWindowsFile(path, expectedSHA256, targetVersion string, prepared *repair.UpdateTransaction) error {
	staging, err := updateCacheDir()
	if err != nil {
		return err
	}
	installDir := currentInstallDir()
	handoff := update.WindowsHandoff{
		InstallerPath:   path,
		InstallerSHA256: expectedSHA256,
		InstallDir:      installDir,
		RelaunchPath:    currentLauncherPath(),
		StagingDir:      staging,
	}
	if installlayout.HasCurrent(installDir) {
		return handoff.StartVersioned(targetVersion)
	}
	if prepared == nil {
		return fmt.Errorf("update: prepared transaction is unavailable")
	}
	return handoff.Start(prepared)
}

func currentExecutablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}

// currentInstallDir is the InstallRoot for updates. For the versioned layout it
// is the directory that owns current.json (not versions/<ver>/). For flat
// installs it is the directory of the running executable.
func currentInstallDir() string {
	exe := currentExecutablePath()
	if exe == "" {
		return ""
	}
	if root, err := installlayout.ResolveInstallRoot(exe); err == nil && root != "" {
		return root
	}
	return filepath.Dir(exe)
}

// archiveSupersededPendingUpdateAfterReady retires a transaction only after the
// current desktop has shown a usable UI. App-bundle recovery handles interrupted
// macOS generations; the versioned-layout branch handles older flat Windows and
// Linux transactions.
func archiveSupersededPendingUpdateAfterReady() (bool, error) {
	exe := currentExecutablePath()
	if exe == "" || version == "" || version == "dev" {
		return false, nil
	}
	if archived, err := repair.ArchiveSupersededPendingAppBundleUpdate(version); err != nil || archived {
		return archived, err
	}
	if runtime.GOOS == "darwin" {
		return false, nil
	}
	root, err := installlayout.ResolveInstallRoot(exe)
	if err != nil {
		return false, err
	}
	ptr, err := installlayout.ReadCurrent(root)
	if err != nil {
		// Package-managed and legacy flat installs have no versioned pointer and
		// therefore are not authorized to retire a transaction.
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	running := strings.TrimSpace(version)
	if !strings.HasPrefix(running, "v") {
		running = "v" + running
	}
	if ptr.ActiveVersion != running {
		return false, fmt.Errorf("active install version %s does not match running version %s", ptr.ActiveVersion, running)
	}
	return repair.ArchiveSupersededPendingFileUpdate(running, root)
}

func capturePendingUpdateHealthIdentity(app *App) {
	if app == nil {
		return
	}
	tx, err := readPendingUpdateForHealth()
	if err != nil || tx == nil || !repair.UpdateVersionsEqual(tx.ToVersion, version) {
		return
	}
	app.healthyUpdateCreatedAt = tx.CreatedAt
	app.healthyUpdateTransactionID = repair.UpdateTransactionID(tx)
}

// refreshPendingUpdateHealthIdentity re-reads the current probationary
// transaction so a user-initiated update can commit health even when the
// process started without a matching identity (for example a historical
// version-prefix mismatch).
func refreshPendingUpdateHealthIdentity(app *App) {
	capturePendingUpdateHealthIdentity(app)
}

// updateSiblingArtifacts lists the packaged binaries an update replaces beside
// the main executable, so PrepareFileUpdate can snapshot the complete release
// unit. Paths that do not exist on disk are skipped by the backup.
func updateSiblingArtifacts() []string {
	dir := currentInstallDir()
	if dir == "" {
		return nil
	}
	paths := releaseUnitPathsFor(dir, runtime.GOOS)
	if len(paths) <= 1 {
		return nil
	}
	return paths[1:]
}

func releaseUnitPathsFor(dir, goos string) []string {
	if dir == "" {
		return nil
	}
	// Versioned-v1 layout: primary is the active desktop under versions/.
	if goos == "windows" && installlayout.HasCurrent(dir) {
		paths := make([]string, 0, 6)
		if desktop, err := installlayout.ActiveDesktopPath(dir); err == nil {
			paths = append(paths, desktop)
		} else {
			paths = append(paths, filepath.Join(dir, "reasonix-desktop.exe"))
		}
		if helper, err := installlayout.ActiveUpdateHelperPath(dir); err == nil {
			paths = append(paths, helper)
		}
		if cli, err := installlayout.ActiveCLIPath(dir); err == nil {
			paths = append(paths, cli)
		}
		for _, name := range []string{"reasonix-launcher.exe", "reasonix-cli.exe", "Reasonix.exe"} {
			paths = append(paths, filepath.Join(dir, name))
		}
		return paths
	}
	names := updateSiblingNames(goos)
	paths := make([]string, 0, len(names)+1)
	switch goos {
	case "linux":
		paths = append(paths, filepath.Join(dir, "reasonix-desktop"))
	case "windows":
		paths = append(paths, filepath.Join(dir, "reasonix-desktop.exe"))
	}
	if len(names) == 0 {
		return paths
	}
	for _, name := range names {
		paths = append(paths, filepath.Join(dir, name))
	}
	return paths
}

func updateSiblingNames(goos string) []string {
	switch goos {
	case "windows":
		// Legacy flat release unit. reasonix-guard.exe may still exist on disk
		// during migration from 1.18–1.19.1; the new layout omits it.
		return []string{"reasonix-guard.exe", "reasonix-launcher.exe", "reasonix-update-helper.exe", "reasonix-cli.exe", "Reasonix.exe"}
	case "linux":
		return []string{"reasonix-guard", "reasonix"}
	default:
		return nil
	}
}

// relaunchThroughLauncher starts the permanent thin launcher (or falls back to
// the running executable). A legacy Guard binary is considered only as a
// one-release migration fallback for flat 1.18-1.19.1 installations.
func relaunchThroughLauncher() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	root := filepath.Dir(exe)
	if resolved, err := installlayout.ResolveInstallRoot(exe); err == nil && resolved != "" {
		root = resolved
	}
	candidates := []string{
		filepath.Join(root, "reasonix-launcher"),
		filepath.Join(root, "Reasonix.exe"),
		filepath.Join(root, "reasonix-guard"), // migration window only
	}
	if runtime.GOOS == "windows" {
		candidates[0] += ".exe"
		candidates[2] += ".exe"
	}
	launcher := exe
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			launcher = path
			break
		}
	}
	args := []string{}
	// Only legacy guard understands "launch --detach"; the thin launcher strips it.
	if strings.Contains(strings.ToLower(filepath.Base(launcher)), "guard") {
		args = []string{"launch", "--detach"}
	}
	cmd := exec.Command(launcher, args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Start()
}

func currentLauncherPath() string {
	exe := currentExecutablePath()
	if exe == "" {
		return ""
	}
	root := filepath.Dir(exe)
	if resolved, err := installlayout.ResolveInstallRoot(exe); err == nil && resolved != "" {
		root = resolved
	}
	for _, name := range []string{"reasonix-launcher.exe", "Reasonix.exe", "reasonix-launcher", "reasonix-guard.exe", "reasonix-guard"} {
		if runtime.GOOS != "windows" && strings.HasSuffix(name, ".exe") {
			continue
		}
		if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") && name != "Reasonix.exe" {
			// Unix names on Windows are unused.
			if !strings.HasSuffix(name, ".exe") {
				continue
			}
		}
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	// Fall through to previous flat-dir behavior for incomplete installs.
	if runtime.GOOS == "windows" {
		guard := filepath.Join(filepath.Dir(exe), "reasonix-guard.exe")
		if _, err := os.Stat(guard); err == nil {
			return guard
		}
	}
	return exe
}
