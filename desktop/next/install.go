package main

import (
	"context"
	"fmt"
	"os"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reasonix/desktop/internal/update"
	"reasonix/internal/netclient"
)

// updateEventName carries install progress to the version panel. It is separate
// from the kernel's event bus on purpose: an install is the shell's business,
// and minting kernel events for it would put host concerns in the transcript.
const updateEventName = "rx:update"

// updateTimeout bounds the whole move. It is generous because the artifact is
// large and a CN route to the CDN can be slow; the user can always close the
// window, which cancels nothing but leaves a verified cache to resume from.
const updateTimeout = 30 * time.Minute

// UpdateProgress is one report from an install in flight.
type UpdateProgress struct {
	Version  string `json:"version"`
	Phase    string `json:"phase"` // downloading | verifying | downloaded | relaunching | error
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	Err      string `json:"err,omitempty"`
}

// GoToVersion installs a published release, forward or back. The pin is set
// before the install, not after: if the machine dies mid-swap, the next launch
// must not helpfully update past the version the user chose.
func (a *App) GoToVersion(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("没有指定要切换到的版本")
	}
	if update.SameVersion(target, version) {
		return nil
	}
	layout := update.Here(studioLine())
	if layout.Root == "" {
		return fmt.Errorf("认不出当前安装位置，无法切换版本")
	}
	dir, err := update.CacheDir()
	if err != nil {
		return a.failUpdate(target, err)
	}
	u, err := studioUpdater(target, dir)
	if err != nil {
		return a.failUpdate(target, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()
	// The shared apply path names the desktop line's release members
	// (ExtractReleaseUnit), so Studio's manifest lists no installable asset yet.
	// Ask it rather than assume: filling platforms is what turns self-update on.
	m, err := u.ManifestFor(ctx, target)
	if err != nil {
		return a.failUpdate(target, err)
	}
	if _, ok := m.Asset(); !ok {
		return a.failUpdate(target, fmt.Errorf("%s 没有 %s 的可安装包，请到 %s 手动下载", target, update.CurrentPlatform(), m.DownloadPage))
	}
	if err := a.PinVersion(target); err != nil {
		return a.failUpdate(target, err)
	}
	inst := update.VersionedInstaller{Layout: layout, Staging: dir, Current: version, Line: studioLine()}
	cached, err := u.DownloadManifest(ctx, m, a.updateReport(target))
	if err != nil {
		return a.failUpdate(target, err)
	}
	if err := inst.Install(ctx, cached); err != nil {
		return a.failUpdate(target, err)
	}
	a.emit(UpdateProgress{Version: target, Phase: "relaunching"})
	a.handOver(layout)
	return nil
}

// handOver ends this process so the installed build can take its place. Windows
// and macOS already have a helper waiting for the exit; Linux replaced the files
// in place, so this process is the one that has to start the new one.
func (a *App) handOver(layout update.Layout) {
	if a.hub != nil {
		a.hub.Shutdown()
	}
	if goruntime.GOOS == "linux" {
		_ = layout.Relaunch()
	}
	os.Exit(0)
}

func (a *App) updateReport(target string) update.Report {
	var total int64
	return update.Report{
		Bytes: func(received, size int64) {
			total = size
			a.emit(UpdateProgress{Version: target, Phase: update.PhaseDownloading, Received: received, Total: size})
		},
		Phase: func(phase string) {
			if phase != update.PhaseDownloading {
				a.emit(UpdateProgress{Version: target, Phase: phase, Received: total, Total: total})
			}
		},
	}
}

func (a *App) failUpdate(target string, err error) error {
	a.emit(UpdateProgress{Version: target, Phase: "error", Err: err.Error()})
	return err
}

func (a *App) emit(p UpdateProgress) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, updateEventName, p)
}

func studioUpdater(target, cacheDir string) (*update.Updater, error) {
	client, err := netclient.NewHTTPClient(proxySpecForUpdates(), netclient.TransportOptions{})
	if err != nil {
		return nil, err
	}
	// Best-effort IPv4 route: a nil fallback just means retries reuse the first.
	v4, _ := netclient.NewHTTPClient(proxySpecForUpdates(), netclient.TransportOptions{ForceIPv4: true})
	return update.New(update.Options{
		Current:  version,
		Pinned:   target,
		HTTP:     client,
		Fallback: v4,
		CacheDir: cacheDir,
		IndexURL: studioCatalog,
		// Go's default user agent is what release-edge bot protection scores
		// worst (#6005), and a 403 there looks like "no versions" to the panel.
		UserAgent:      fmt.Sprintf("Reasonix-Studio/%s (%s/%s)", version, goruntime.GOOS, goruntime.GOARCH),
		AttemptTimeout: 5 * time.Second,
	}), nil
}
