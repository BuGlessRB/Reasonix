package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"reasonix/internal/remote/sftpfs"
)

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

	switch strategy {
	case InstallNever:
		return fail(errors.New("bootstrap: reasonix not found on remote and serve_install = never"))
	case InstallNPM:
		b, v, nerr := installViaNPM(ctx, conn, target, opts.MinVersion)
		if nerr != nil {
			return fail(nerr)
		}
		return b, v, nil
	case InstallUpload:
		b, v, uerr := installViaUpload(ctx, conn, target, fs, opts, home, goos, goarch, uploaded)
		if uerr != nil {
			return fail(uerr)
		}
		return b, v, nil
	default: // auto: try npm, packaged same-platform upload, then verified release upload
		if b, v, nerr := installViaNPM(ctx, conn, target, opts.MinVersion); nerr == nil {
			return b, v, nil
		} else {
			attempts := []error{nerr}
			if opts.LocalBinary != "" && opts.LocalGOOS == goos && opts.LocalGOARCH == goarch {
				if b, v, uploadErr := installViaUpload(ctx, conn, target, fs, opts, home, goos, goarch, uploaded); uploadErr == nil {
					return b, v, nil
				} else {
					attempts = append(attempts, uploadErr)
				}
			} else if opts.LocalBinary == "" {
				attempts = append(attempts, errors.New("bootstrap: no local Reasonix CLI is available for upload"))
			} else {
				attempts = append(attempts, fmt.Errorf("bootstrap: local binary is %s/%s but remote is %s/%s", opts.LocalGOOS, opts.LocalGOARCH, goos, goarch))
			}
			if opts.FetchBinary != nil {
				binary, fetchErr := opts.FetchBinary(ctx, opts.ProductVersion, goos, goarch)
				if fetchErr == nil {
					if b, v, uploadErr := installBinaryBytes(ctx, conn, target, fs, binary, opts.MinVersion, home, uploaded); uploadErr == nil {
						return b, v, nil
					} else {
						attempts = append(attempts, uploadErr)
					}
				} else {
					attempts = append(attempts, fmt.Errorf("bootstrap: fetch official %s/%s CLI: %w", goos, goarch, fetchErr))
				}
			}
			return fail(fmt.Errorf("bootstrap: automatic install failed: %w", errors.Join(attempts...)))
		}
	}
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
		return "", "", fmt.Errorf("bootstrap: npm install: %w", err)
	}
	if res.ExitCode != 0 {
		return "", "", fmt.Errorf("bootstrap: npm install failed: %s", tail(res.Stdout, 400))
	}
	// npm may install outside the login PATH; probe npm prefix explicitly.
	loc, ver := locate(ctx, conn, target, "", minVersion)
	if loc == "" {
		return "", "", fmt.Errorf("bootstrap: reasonix not found after npm install (check remote PATH / npm prefix)")
	}
	return loc, ver, nil
}

// installViaUpload uploads the local reasonix binary when the remote platform
// matches the local one. A differing platform is served by the verified
// release download instead, which carries every target the line publishes.
func installViaUpload(ctx context.Context, conn Conn, target remoteOS, fs *sftpfs.FS, opts Options, home, goos, goarch, uploaded string) (bin, version string, err error) {
	if opts.LocalBinary == "" {
		return "", "", fmt.Errorf("bootstrap: upload strategy needs the local reasonix binary path")
	}
	if opts.LocalGOOS != goos || opts.LocalGOARCH != goarch {
		return "", "", fmt.Errorf("bootstrap: cannot upload: local binary is %s/%s but remote is %s/%s; use serve_install = npm",
			opts.LocalGOOS, opts.LocalGOARCH, goos, goarch)
	}
	data, rerr := os.ReadFile(opts.LocalBinary)
	if rerr != nil {
		return "", "", fmt.Errorf("bootstrap: read local binary: %w", rerr)
	}
	return installBinaryBytes(ctx, conn, target, fs, data, opts.MinVersion, home, uploaded)
}

func installBinaryBytes(ctx context.Context, conn Conn, target remoteOS, fs *sftpfs.FS, data []byte, minVersion, home, uploaded string) (bin, version string, err error) {
	if len(data) == 0 {
		return "", "", fmt.Errorf("bootstrap: downloaded binary is empty")
	}
	if err := fs.MkdirAll(ctx, dirOf(uploaded)); err != nil {
		return "", "", err
	}
	if err := fs.WriteFileAtomic(ctx, uploaded, data, 0o755); err != nil {
		return "", "", fmt.Errorf("bootstrap: upload binary: %w", err)
	}
	loc, ver := locate(ctx, conn, target, uploaded, minVersion)
	if loc == "" {
		return "", "", fmt.Errorf("bootstrap: uploaded binary not runnable on remote")
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
