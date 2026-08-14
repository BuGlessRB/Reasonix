package update

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// ProgressFunc reports bytes received against the total, for a progress bar.
type ProgressFunc func(received, total int64)

// Installer applies a verified artifact and hands over to the relaunched build.
// It stays with the host on purpose: applying is where an update stops being
// bytes and becomes this machine's install layout — a package manager, an app
// bundle, or a helper process that has to outlive the process starting it.
type Installer interface {
	Install(ctx context.Context, c Cached) error
}

// Options configures an Updater. The pin is passed in rather than read here:
// it is a user setting the shells own, and keeping it out makes the policy
// testable without a config file.
type Options struct {
	Current  string       // running version
	Pinned   string       // release the user pinned, or ""
	IndexURL string       // catalog; empty uses the published one
	HTTP     *http.Client // carries the caller's proxy and timeouts
	Fallback *http.Client // IPv4-pinned route, tried from the second attempt
	CacheDir string       // where a verified artifact waits for its install
	Kind     string       // artifact this install applies; empty is the tarball
	// UserAgent identifies updater traffic. Go's default is what edge bot
	// protection scores worst (#6005), so a build that leaves this empty is
	// asking to be 403'd by its own CDN.
	UserAgent string
	// AttemptTimeout bounds one manifest or signature fetch so a stalled
	// primary route cannot hold the budget the IPv4 fallback needs. Downloads
	// are deliberately unbounded; artifacts are large.
	AttemptTimeout time.Duration
}

// Updater answers what is published and what that means for this machine.
type Updater struct {
	opts Options
}

// New returns an Updater for the running build.
func New(opts Options) *Updater { return &Updater{opts: opts} }

// Status is what a version panel renders and what an auto-updater decides on.
// Entries is the whole catalog, newest first, because a rollback needs the
// versions behind the running one as much as an update needs the one ahead.
type Status struct {
	Current   string
	Latest    string
	Pinned    string
	Available bool
	Newer     bool
	AutoOK    bool // may be installed without asking; false while pinned
	StalePin  bool
	Entries   []IndexEntry
}

// Check reads the catalog and applies the pin rule. A failure to reach the
// catalog is returned with the status still filled in as far as it got: which
// version is running is a local fact and must survive a network error.
func (u *Updater) Check(ctx context.Context) (Status, error) {
	st := Status{Current: u.opts.Current, Pinned: strings.TrimSpace(u.opts.Pinned)}
	idx, err := FetchIndex(ctx, u.opts.HTTP, u.opts.IndexURL)
	if err != nil {
		return st, err
	}
	st.Entries = idx.Versions
	for _, e := range idx.Versions {
		if st.Latest == "" || CompareVersions(e.Version, st.Latest) > 0 {
			st.Latest = e.Version
		}
	}
	offer := OfferFor(st.Current, st.Latest, st.Pinned)
	st.Available, st.Newer, st.AutoOK, st.StalePin = offer.Available, offer.Newer, offer.AutoInstall, offer.StalePin
	return st, nil
}

// Manifest resolves one catalog entry to its immutable manifest, the single
// source for that version's assets and signatures.
func (u *Updater) Manifest(ctx context.Context, e IndexEntry) (*Manifest, error) {
	return FetchManifestAt(ctx, u.opts.HTTP, e.Manifest)
}
