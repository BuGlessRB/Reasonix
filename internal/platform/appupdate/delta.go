package appupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"time"

	"reasonix/internal/base/netclient"
	"reasonix/internal/platform/delta"
	"reasonix/internal/platform/update"
)

const (
	deltaParallel  = 8
	deltaAttempts  = 3
	maxIndexBytes  = 16 << 20
	maxStoredChunk = 1 << 20
	deltaFetchWait = 30 * time.Second
)

// errDeltaMismatch is a published delta that does not belong to the release
// asked for, or whose index is not the one the manifest names.
var errDeltaMismatch = errors.New("appupdate: the published delta does not match the release")

// tryDelta installs target from the chunks this install lacks and reports
// whether it did. Everything before the handoff only reads the install and
// writes the cache, so any failure there leaves the full package to do the
// job, which is why it is logged rather than returned.
func (c *capability) tryDelta(ctx context.Context, install update.Install, target, cacheDir string, m *update.Manifest) bool {
	d, ok := m.Deltas[update.CurrentPlatform()]
	if !ok || !update.TreeHandoffSupported() || install.Layout.Root == "" || c.opts.Application.PID <= 0 {
		return false
	}
	h, err := c.stageDelta(ctx, install, target, cacheDir, d)
	if err == nil {
		var self string
		if self, err = os.Executable(); err == nil {
			err = update.StartTreeHandoff(h, self)
		}
	}
	if err != nil {
		slog.Warn("appupdate: chunked update unavailable, downloading the full package", "target", target, "err", err)
		return false
	}
	return true
}

func (c *capability) stageDelta(ctx context.Context, install update.Install, target, cacheDir string, d update.Delta) (update.TreeHandoff, error) {
	client, err := netclient.NewHTTPClient(update.ProxySpec(), netclient.TransportOptions{})
	if err != nil {
		return update.TreeHandoff{}, err
	}
	get := func(ctx context.Context, url string, limit int64) ([]byte, error) {
		return c.fetch(ctx, client, url, limit)
	}
	packed, err := get(ctx, d.Index.URL, maxIndexBytes)
	if err != nil {
		return update.TreeHandoff{}, err
	}
	sig, err := get(ctx, d.Index.Sig, 64<<10)
	if err != nil {
		return update.TreeHandoff{}, err
	}
	if s := sha256.Sum256(packed); hex.EncodeToString(s[:]) != d.Index.SHA256 {
		return update.TreeHandoff{}, fmt.Errorf("%w: index digest", errDeltaMismatch)
	}
	if err := update.Verify(packed, sig); err != nil {
		return update.TreeHandoff{}, err
	}
	x, err := delta.UnpackIndex(packed)
	if err != nil {
		return update.TreeHandoff{}, err
	}
	if !update.SameVersion(x.Version, target) || x.Platform != update.CurrentPlatform() {
		return update.TreeHandoff{}, fmt.Errorf("%w: index is %s for %s", errDeltaMismatch, x.Version, x.Platform)
	}
	plan, err := delta.PlanFrom(x, install.Layout.Root)
	if err != nil {
		return update.TreeHandoff{}, err
	}
	work := filepath.Join(cacheDir, "delta")
	chunks := filepath.Join(work, "chunks")
	fetchChunk := func(ctx context.Context, hash string) ([]byte, error) {
		return get(ctx, d.Chunks+"/"+delta.ObjectName(hash), maxStoredChunk)
	}
	progress := func(done, total int64) {
		c.install.set(update.Progress{Version: target, Phase: update.PhaseDownloading, Received: done, Total: total})
	}
	if err := delta.Fetch(ctx, plan.Missing, fetchChunk, chunks, deltaParallel, progress); err != nil {
		return update.TreeHandoff{}, err
	}
	release := filepath.Join(work, x.Version)
	staging, backup := filepath.Join(release, "tree"), filepath.Join(release, "backup")
	for _, dir := range []string{staging, backup} {
		if err := os.RemoveAll(dir); err != nil {
			return update.TreeHandoff{}, err
		}
	}
	c.install.set(update.Progress{Version: target, Phase: update.PhaseVerifying})
	if err := delta.Assemble(x, plan, install.Layout.Root, chunks, staging); err != nil {
		return update.TreeHandoff{}, err
	}
	_ = os.RemoveAll(chunks)
	h := update.TreeHandoff{
		Version: x.Version, InstallDir: install.Layout.Root, StagingDir: staging, BackupDir: backup,
		Relaunch: install.Layout.Launcher, WaitPIDs: []int{c.opts.Application.PID, os.Getpid()},
	}
	if h.Relaunch == "" {
		h.Relaunch = install.Layout.Executable
	}
	for _, f := range x.Files {
		h.Files = append(h.Files, update.StagedFile{Path: f.Path, SHA256: f.SHA256})
	}
	return h, nil
}

// fetch reads one mirror object whole, retrying a transient failure; limit
// bounds what a misbehaving server can make this process hold.
func (c *capability) fetch(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	var err error
	for attempt := range deltaAttempts {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		var b []byte
		if b, err = c.fetchOnce(ctx, client, url, limit); err == nil || ctx.Err() != nil {
			return b, err
		}
	}
	return nil, err
}

func (c *capability) fetchOnce(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, deltaFetchWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fmt.Sprintf("Reasonix-Studio/%s (%s/%s)", c.opts.Running, goruntime.GOOS, goruntime.GOARCH))
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("appupdate: %s answered %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("appupdate: %s is larger than %d bytes", url, limit)
	}
	return b, nil
}
