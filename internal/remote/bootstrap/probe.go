package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// routeDownload is the verified-release upload the auto strategy falls back
// to. It is not a serve_install value — nobody can ask for it alone.
const routeDownload = "download"

// Report is what one machine can and cannot do, read in a single session. A
// cold connect walks these one at a time and stops at the first that fails, so
// every failed attempt teaches the reader exactly one missing piece; this
// answers all of them at once.
type Report struct {
	OS, Arch, Home string
	// Kernel is a reasonix already there that this caller could drive.
	Kernel, Version string
	// Outdated is one that is there and below the floor — having an old one is
	// a different next move from having none.
	Outdated string
	// NPM is npm's own version over there; empty means it would not run.
	NPM    string
	Routes []Route
}

// Route is one way a kernel could reach that machine, and what stands in the
// way when something does. Err carries this package's own sentinels, so a
// probe and a failed connect name the same obstacle the same way.
type Route struct {
	Name string // npm | upload | download | never
	Err  error  // nil when the route is open
}

// OK reports whether this route could be taken.
func (r Route) OK() bool { return r.Err == nil }

// Ready reports whether a connect would find or install a kernel. It promises
// a route exists, never that it will land: npm can still refuse to write, and
// only trying finds that out.
func (r Report) Ready() bool {
	if r.Kernel != "" {
		return true
	}
	for _, route := range r.Routes {
		if route.OK() {
			return true
		}
	}
	return false
}

// Probe reads everything a first connect depends on, and changes nothing over
// there.
func Probe(ctx context.Context, conn Conn, opts Options) (Report, error) {
	fs, err := conn.SFTP()
	if err != nil {
		return Report{}, err
	}
	target, goos, goarch, home, err := remoteFor(ctx, conn, fs)
	if err != nil {
		return Report{}, err
	}
	// Home as that machine spells it: the file layer's /C:/Users/... is not a
	// path anybody there would type.
	rep := Report{OS: goos, Arch: goarch, Home: target.NativePath(home)}
	found := probeBinaries(ctx, conn, target, uploadedBinPath(home, target.Executable()))
	for _, c := range found {
		if c.usable(opts.MinVersion) {
			rep.Kernel, rep.Version = c.path, c.version
			break
		}
	}
	rep.Outdated = outdated(found, opts.MinVersion)
	if res, execErr := conn.Exec(ctx, target.NPMVersion()); execErr == nil && res.ExitCode == 0 {
		rep.NPM = strings.TrimSpace(string(res.Stdout))
	}
	rep.Routes = routesFor(rep, opts, goos, goarch)
	return rep, nil
}

// routesFor mirrors what ensureBinary would try, in the order it would try
// them. The two have to agree: a probe reporting a route the installer never
// takes is worse than no probe at all.
func routesFor(rep Report, opts Options, goos, goarch string) []Route {
	strategy := opts.Install
	if strategy == "" {
		strategy = InstallAuto
	}
	if strategy == InstallNever {
		return []Route{{Name: InstallNever, Err: ErrInstallDisabled}}
	}

	npm := Route{Name: InstallNPM}
	if rep.NPM == "" {
		npm.Err = ErrNPMUnavailable
	}
	upload := Route{Name: InstallUpload}
	switch {
	case opts.LocalBinary == "":
		upload.Err = fmt.Errorf("%w: no local Reasonix CLI to upload", ErrPlatformMismatch)
	case opts.LocalGOOS != goos || opts.LocalGOARCH != goarch:
		upload.Err = fmt.Errorf("%w: local is %s/%s, remote is %s/%s",
			ErrPlatformMismatch, opts.LocalGOOS, opts.LocalGOARCH, goos, goarch)
	}

	switch strategy {
	case InstallNPM:
		return []Route{npm}
	case InstallUpload:
		return []Route{upload}
	}
	download := Route{Name: routeDownload}
	if opts.FetchBinary == nil {
		// No sentinel: this is the caller not wiring a fetcher, not anything
		// about the machine, and it never happens in a shipped build.
		download.Err = errors.New("this build cannot fetch a release")
	}
	return []Route{npm, upload, download}
}
