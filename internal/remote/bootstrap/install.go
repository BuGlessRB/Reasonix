package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"reasonix/internal/remote/sftpfs"
)

// autoRouteOrder is the order auto tries a machine in, and the only place that
// order is written — routesFor reports this same list. The remote fetching its
// own release is first because it spends nobody's uplink; npm is last because
// it needs Node on the far side to deliver the identical static binary, and
// stays only because a network mirroring npm while throttling GitHub is real.
var autoRouteOrder = []string{routeRemoteFetch, InstallUpload, routeDownload, InstallNPM}

// ensureBinary resolves a reasonix on the remote host that this caller can
// drive, installing one per the strategy when what is there will not do.
// Present-but-below-the-floor is not the same answer as absent, and the one it
// returns says which: an upgrade over there and an install are different moves.
func ensureBinary(ctx context.Context, conn Conn, target remoteOS, fs *sftpfs.FS, opts Options, home, goos, goarch string, paths StatePaths) (bin, version string, err error) {
	uploaded := uploadedBinPath(home, target.Executable())
	found := probeBinaries(ctx, conn, target, uploaded)
	for _, c := range found {
		if c.usable(opts.MinVersion) {
			return c.path, c.version, nil
		}
	}
	// What the machine already had, if an install cannot better it. Held now
	// because the probe below runs against a tree the install may have changed.
	stale := outdated(found, opts.MinVersion)
	fail := func(err error) (string, string, error) {
		if stale != "" {
			return "", "", &KernelTooOldError{Found: stale, Need: opts.MinVersion, Err: err}
		}
		return "", "", err
	}

	strategy := opts.Install
	if strategy == "" {
		strategy = InstallAuto
	}
	opts.progress("install", strategy)
	if strategy == InstallNever {
		return fail(ErrInstallDisabled)
	}

	routes := []string{strategy}
	if strategy == InstallAuto {
		routes = autoRouteOrder
	}
	var attempts []error
	for _, route := range routes {
		b, v, rerr := runRoute(ctx, conn, target, fs, opts, route, home, goos, goarch, uploaded)
		if rerr == nil {
			return b, v, nil
		}
		attempts = append(attempts, rerr)
	}
	// One route asked for by name failed for its own reason, which the caller
	// should see. Every route failing is a different fact about the machine.
	if len(attempts) == 1 {
		return fail(attempts[0])
	}
	return fail(fmt.Errorf("%w: %w", ErrNoInstallPath, errors.Join(attempts...)))
}

func runRoute(ctx context.Context, conn Conn, target remoteOS, fs *sftpfs.FS, opts Options, route, home, goos, goarch, uploaded string) (bin, version string, err error) {
	switch route {
	case routeRemoteFetch:
		return installViaRemoteFetch(ctx, conn, target, opts, home, goos, goarch, uploaded)
	case InstallNPM:
		return installViaNPM(ctx, conn, target, opts.MinVersion)
	case InstallUpload:
		return installViaUpload(ctx, conn, target, fs, opts, home, goos, goarch, uploaded)
	case routeDownload:
		return installViaDownload(ctx, conn, target, fs, opts, home, goos, goarch, uploaded)
	}
	return "", "", fmt.Errorf("bootstrap: unknown install route %q", route)
}

// installViaRemoteFetch has the machine download its own kernel. Nothing
// crosses the SSH connection but the command and its result — the archive
// travels over that host's own link, which is usually the fatter one.
func installViaRemoteFetch(ctx context.Context, conn Conn, target remoteOS, opts Options, home, goos, goarch, uploaded string) (bin, version string, err error) {
	if opts.ResolveDownload == nil {
		return "", "", ErrRemoteFetchUnavailable
	}
	d, err := opts.ResolveDownload(ctx, opts.ProductVersion, goos, goarch)
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", ErrRemoteFetchUnavailable, err)
	}
	res, err := conn.Exec(ctx, target.Fetch(d, dirOf(uploaded), uploaded))
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", ErrRemoteFetchUnavailable, err)
	}
	if res.ExitCode != 0 {
		return "", "", fmt.Errorf("%w: %s", ErrRemoteFetchUnavailable, tail(res.Stdout, 400))
	}
	loc, ver := locate(ctx, conn, target, uploaded, opts.MinVersion)
	if loc == "" {
		return "", "", ErrBinaryNotRunnable
	}
	return loc, ver, nil
}

// installViaDownload fetches the release here and pushes it over SFTP. The
// route for a machine that cannot reach the release itself.
func installViaDownload(ctx context.Context, conn Conn, target remoteOS, fs *sftpfs.FS, opts Options, home, goos, goarch, uploaded string) (bin, version string, err error) {
	if opts.FetchBinary == nil {
		// Not a sentinel: this is the caller wiring no fetcher, nothing about
		// the machine, and it never happens in a shipped build.
		return "", "", errors.New("bootstrap: this build cannot fetch a release")
	}
	binary, err := opts.FetchBinary(ctx, opts.ProductVersion, goos, goarch)
	if err != nil {
		return "", "", fmt.Errorf("bootstrap: fetch official %s/%s CLI: %w", goos, goarch, err)
	}
	return installBinaryBytes(ctx, conn, target, fs, binary, opts.MinVersion, home, uploaded)
}

// candidate is one reasonix the probe found, with what it answered about
// itself. Two things make one usable: the --port-file flag the launch is
// written against, and a version at or above the caller's floor.
type candidate struct {
	path     string
	version  string
	portFile bool
}

func (c candidate) usable(minVersion string) bool {
	return c.path != "" && c.portFile && meetsMinVersion(c.version, minVersion)
}

// locate returns the first reasonix on the remote that this caller can drive.
// A binary that is present but below the floor is not one of them: it would
// launch, answer, and then refuse the only calls the caller had for it.
func locate(ctx context.Context, conn Conn, target remoteOS, uploaded, minVersion string) (bin, version string) {
	for _, c := range probeBinaries(ctx, conn, target, uploaded) {
		if c.usable(minVersion) {
			return c.path, c.version
		}
	}
	return "", ""
}

func probeBinaries(ctx context.Context, conn Conn, target remoteOS, uploaded string) []candidate {
	res, err := conn.Exec(ctx, target.Locate(uploaded))
	if err != nil {
		return nil
	}
	return parseCandidates(string(res.Stdout))
}

// parseCandidates reads the probe's records. `bin` opens one and the lines
// after it fill it, so a machine with several reasonix installs comes back as
// several candidates in the order the probe looked.
func parseCandidates(out string) []candidate {
	var found []candidate
	for line := range strings.SplitSeq(out, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), " ")
		if val = strings.TrimSpace(val); !ok || val == "" {
			continue
		}
		if key == "bin" {
			found = append(found, candidate{path: val})
			continue
		}
		if len(found) == 0 {
			continue
		}
		switch key {
		case "ver":
			if v, verr := ParseVersion(val); verr == nil {
				found[len(found)-1].version = v
			}
		case "flag":
			found[len(found)-1].portFile = val == "yes"
		}
	}
	return found
}

// outdated is the version of the newest reasonix that was found and turned
// down for its age. It is what separates "this machine has none" from "this
// machine has one from another line", which are different next moves.
func outdated(found []candidate, minVersion string) string {
	newest := ""
	for _, c := range found {
		if c.version == "" || meetsMinVersion(c.version, minVersion) {
			continue
		}
		if newest == "" || CompareVersions(c.version, newest) > 0 {
			newest = c.version
		}
	}
	return newest
}

func installViaNPM(ctx context.Context, conn Conn, target remoteOS, minVersion string) (bin, version string, err error) {
	res, err := conn.Exec(ctx, "npm i -g reasonix 2>&1")
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", ErrNPMUnavailable, err)
	}
	if res.ExitCode != 0 {
		return "", "", fmt.Errorf("%w: %s", ErrNPMUnavailable, tail(res.Stdout, 400))
	}
	// npm may install outside the login PATH; probe npm prefix explicitly.
	loc, ver := locate(ctx, conn, target, "", minVersion)
	if loc == "" {
		return "", "", ErrNPMOutsidePath
	}
	return loc, ver, nil
}

// installViaUpload uploads the local reasonix binary when the remote platform
// matches the local one. A differing platform is served by the verified
// release download instead, which carries every target the line publishes.
func installViaUpload(ctx context.Context, conn Conn, target remoteOS, fs *sftpfs.FS, opts Options, home, goos, goarch, uploaded string) (bin, version string, err error) {
	if opts.LocalBinary == "" {
		return "", "", fmt.Errorf("%w: no local Reasonix CLI to upload", ErrPlatformMismatch)
	}
	if opts.LocalGOOS != goos || opts.LocalGOARCH != goarch {
		return "", "", fmt.Errorf("%w: local is %s/%s, remote is %s/%s",
			ErrPlatformMismatch, opts.LocalGOOS, opts.LocalGOARCH, goos, goarch)
	}
	data, rerr := os.ReadFile(opts.LocalBinary)
	if rerr != nil {
		return "", "", fmt.Errorf("bootstrap: read local binary: %w", rerr)
	}
	return installBinaryBytes(ctx, conn, target, fs, data, opts.MinVersion, home, uploaded)
}

func installBinaryBytes(ctx context.Context, conn Conn, target remoteOS, fs *sftpfs.FS, data []byte, minVersion, home, uploaded string) (bin, version string, err error) {
	if len(data) == 0 {
		return "", "", fmt.Errorf("%w: the download was empty", ErrBinaryNotRunnable)
	}
	if err := fs.MkdirAll(ctx, dirOf(uploaded)); err != nil {
		return "", "", err
	}
	if err := fs.WriteFileAtomic(ctx, uploaded, data, 0o755); err != nil {
		return "", "", fmt.Errorf("bootstrap: upload binary: %w", err)
	}
	loc, ver := locate(ctx, conn, target, uploaded, minVersion)
	if loc == "" {
		return "", "", ErrBinaryNotRunnable
	}
	return loc, ver, nil
}

func dirOf(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}
	return "."
}

func tail(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		return "..." + s[len(s)-n:]
	}
	return s
}
