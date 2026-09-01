package appupdate

import (
	"context"
	"errors"
	"fmt"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"reasonix/internal/netclient"
	"reasonix/internal/update"
)

// installTimeout bounds the whole move. It is generous because the artifact is
// large and a CN route to the CDN can be slow; a caller that gives up leaves a
// verified cache to resume from.
const installTimeout = 30 * time.Minute

// What a caller tells apart. Each is a different thing to do about it: name a
// version, install this build somewhere the updater recognizes, or wait.
var (
	ErrNoTarget        = errors.New("appupdate: no version was named")
	ErrUnknownInstall  = errors.New("appupdate: cannot tell where this build is installed")
	ErrInstallInFlight = errors.New("appupdate: an install is already running")
)

// installState is the one install this application may have in flight. It is a
// sub-state rather than fields on the capability because its whole content has
// one lifetime: written by the goroutine doing the move, read by every panel
// asking what that move is doing.
type installState struct {
	mu       sync.Mutex
	progress update.Progress
}

// begin claims the slot, or reports that something else holds it. Claiming and
// checking are one act: two callers that checked first would both start.
func (s *installState) begin(target string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.progress.Running() {
		return false
	}
	s.progress = update.Progress{Version: target, Phase: update.PhaseDownloading}
	return true
}

func (s *installState) set(p update.Progress) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress = p
}

func (s *installState) read() update.Progress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress
}

// InstallProgress answers what the move is doing. It is a projection: a caller
// that missed a frame is restored by this read, and the last thing an install
// does, ending this process, is not a frame anybody receives.
func (c *capability) InstallProgress() update.Progress {
	return c.install.read()
}

// StartInstall begins moving this application to target, forward or back, and
// returns once it is under way rather than once it is done. It cannot report
// the end: an install that worked finishes by ending this process, so what a
// caller watches is InstallProgress and then the application coming back.
func (c *capability) StartInstall(install update.Install, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return ErrNoTarget
	}
	if update.SameVersion(target, c.opts.Running) {
		return nil
	}
	if install.Layout.Root == "" {
		return ErrUnknownInstall
	}
	if !c.install.begin(target) {
		return ErrInstallInFlight
	}
	// Detached from the request that asked for it: the caller is answered now,
	// and a move that outlives its HTTP context is the point rather than a leak.
	go c.move(install, target)
	return nil
}

func (c *capability) move(install update.Install, target string) {
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	switch err := c.apply(ctx, install, target); {
	case update.DebAuthCancelled(err):
		// A dismissed prompt is a decision, not a failure. The verified cache
		// stays, so a retry costs no download.
		c.install.set(update.Progress{Version: target, Phase: update.PhaseCached})
		return
	case err != nil:
		c.install.set(update.Progress{Version: target, Phase: update.PhaseFailed, Err: err.Error()})
		return
	}
	c.install.set(update.Progress{Version: target, Phase: update.PhaseRelaunching})
	c.handOver(ctx)
}

// apply moves this application, and returns only if it did not.
func (c *capability) apply(ctx context.Context, install update.Install, target string) error {
	dir, err := update.CacheDir()
	if err != nil {
		return err
	}
	u, err := c.updater(target, dir, "")
	if err != nil {
		return err
	}
	m, err := u.ManifestFor(ctx, target)
	if err != nil {
		return err
	}
	// A dpkg install upgrades through its package, or apt and the filesystem
	// end up disagreeing. It is also the only channel carrying the SPA tree:
	// the versioned layout stages single files.
	if _, ok := m.NativePackage(); ok && c.opts.Line.OwnsInstalledPath(install.Layout.Executable) {
		return c.applyNativePackage(ctx, dir, target, m)
	}
	if _, ok := m.Asset(); !ok {
		return fmt.Errorf("appupdate: %s has no installable package for %s; download it from %s", target, update.CurrentPlatform(), m.DownloadPage)
	}
	// Pinned before the install, not after: if the machine dies mid-swap, the
	// next launch must not helpfully update past the version the user chose.
	if err := update.Pin(target); err != nil {
		return err
	}
	cached, err := u.DownloadManifest(ctx, m, c.report(target))
	if err != nil {
		return err
	}
	return c.applyDownloaded(ctx, install, target, dir, cached)
}

// applyNativePackage hands a verified .deb to Polkit. The updater is rebuilt
// declaring the deb kind so it resolves the package rather than the tarball:
// handing a dpkg install the portable archive would leave apt and the
// filesystem disagreeing about what is installed.
func (c *capability) applyNativePackage(ctx context.Context, cacheDir, target string, m *update.Manifest) error {
	u, err := c.updater(target, cacheDir, update.KindDeb)
	if err != nil {
		return err
	}
	if err := update.Pin(target); err != nil {
		return err
	}
	cached, err := u.DownloadManifest(ctx, m, c.report(target))
	if err != nil {
		return err
	}
	if cached.SignaturePath == "" {
		return fmt.Errorf("appupdate: the package for %s carries no signature", target)
	}
	c.install.set(update.Progress{Version: target, Phase: update.PhaseAuthorizing})
	return c.opts.Line.InstallDeb(cached.Path, cached.SignaturePath, func(phase string) {
		c.install.set(update.Progress{Version: target, Phase: phase})
	})
}

// handOver ends this application so the installed build can take its place. All
// three acts stay the owner's: what releases the application, what starts its
// successor, and what ends it are not one thing on every platform.
func (c *capability) handOver(ctx context.Context) {
	_ = c.opts.Owner.PrepareForUpdate(ctx)
	_ = c.opts.Owner.RelaunchAfterUpdate(ctx)
	c.opts.Owner.EndApplication(ctx)
}

func (c *capability) report(target string) update.Report {
	var total int64
	return update.Report{
		Bytes: func(received, size int64) {
			total = size
			c.install.set(update.Progress{Version: target, Phase: update.PhaseDownloading, Received: received, Total: size})
		},
		Phase: func(phase string) {
			if phase != update.PhaseDownloading {
				c.install.set(update.Progress{Version: target, Phase: phase, Received: total, Total: total})
			}
		},
	}
}

// updater names which artifact this install can apply. An empty kind resolves
// the portable asset; KindDeb resolves the package channel.
func (c *capability) updater(target, cacheDir, kind string) (*update.Updater, error) {
	client, err := netclient.NewHTTPClient(update.ProxySpec(), netclient.TransportOptions{})
	if err != nil {
		return nil, err
	}
	// Best-effort IPv4 route: a nil fallback just means retries reuse the first.
	v4, _ := netclient.NewHTTPClient(update.ProxySpec(), netclient.TransportOptions{ForceIPv4: true})
	return update.New(update.Options{
		Current:  c.opts.Running,
		Pinned:   target,
		HTTP:     client,
		Fallback: v4,
		CacheDir: cacheDir,
		IndexURL: update.StudioCatalog,
		Kind:     kind,
		// Go's default user agent is what release-edge bot protection scores
		// worst (#6005), and a 403 there looks like "no versions" to the panel.
		UserAgent:      fmt.Sprintf("Reasonix-Studio/%s (%s/%s)", c.opts.Running, goruntime.GOOS, goruntime.GOARCH),
		AttemptTimeout: 5 * time.Second,
	}), nil
}
