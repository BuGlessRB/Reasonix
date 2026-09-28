package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"reasonix/internal/ext/installsource"
)

var (
	// ErrUnpinned: the approved version carries no content digest, so nothing
	// could tell the reviewed material from whatever the source holds today.
	ErrUnpinned = errors.New("market: approved version is not pinned to reviewed content")
	// ErrBadSource: the approved row names something this side will not fetch —
	// a local path, a plain-http address, an unknown kind.
	ErrBadSource = errors.New("market: approved version names an uninstallable source")
	// ErrVersionChanged: the approved version is no longer the one the person
	// looked at.
	ErrVersionChanged = errors.New("market: approved version changed")
)

// Request is one plan or install of a listed package. Version is the approved
// version the person was shown; empty accepts whichever is approved now, which
// only a preview does.
type Request struct {
	Slug    string `json:"slug"`
	Version string `json:"version,omitempty"`
	PlanID  string `json:"planId,omitempty"`
	Replace bool   `json:"replace,omitempty"`
}

// Service joins the registry to install_source.
type Service struct {
	Registry Registry
	// Home is the Reasonix home the ledger lives under.
	Home string
	// NewInstaller returns install_source. It must require an approved plan so
	// an apply without a planId answers with the plan instead of installing.
	NewInstaller func() *installsource.Tool
	Now          func() time.Time
}

// Outcome is install_source's own answer plus which approved version it was.
type Outcome struct {
	Fields  map[string]json.RawMessage
	Version Version
}

// Plan previews the approved version of slug without writing anything.
func (s *Service) Plan(ctx context.Context, req Request) (Outcome, error) {
	return s.run(ctx, req, false)
}

// Install applies the plan req.PlanID names and records what landed.
func (s *Service) Install(ctx context.Context, req Request) (Outcome, error) {
	return s.run(ctx, req, true)
}

func (s *Service) run(ctx context.Context, req Request, apply bool) (Outcome, error) {
	detail, err := s.Registry.Detail(ctx, req.Slug)
	if err != nil {
		return Outcome{}, err
	}
	v, err := installable(detail)
	if err != nil {
		return Outcome{}, err
	}
	if req.Version != "" && req.Version != v.Version {
		return Outcome{Version: v}, fmt.Errorf("%w: shown %s, approved now %s", ErrVersionChanged, req.Version, v.Version)
	}
	body := map[string]any{
		"source":       v.Source,
		"kind":         detail.Package.Kind,
		"scope":        "global",
		"mode":         "copy",
		"replace":      req.Replace,
		"apply":        apply,
		"expectDigest": v.ContentHash,
	}
	if apply {
		body["planId"] = strings.TrimSpace(req.PlanID)
	}
	raw, _ := json.Marshal(body)
	out, err := s.NewInstaller().Execute(ctx, raw)
	if err != nil {
		return Outcome{Version: v}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &fields); err != nil {
		return Outcome{Version: v}, err
	}
	if apply {
		if items := doneItems(fields["actions"]); len(items) > 0 {
			rec := Record{Slug: detail.Package.Slug, Kind: detail.Package.Kind, Version: v.Version,
				ContentHash: v.ContentHash, Items: items, At: s.now().UTC().Format(time.RFC3339)}
			if err := saveRecord(s.Home, rec); err != nil {
				fields["ledgerError"], _ = json.Marshal(err.Error())
			}
		}
	}
	return Outcome{Fields: fields, Version: v}, nil
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// installable answers the approved version when it can be installed as
// reviewed, and why not otherwise.
func installable(d Detail) (Version, error) {
	v := d.Approved
	if v == nil {
		return Version{}, fmt.Errorf("%w: no row for approved version %q", ErrUnpinned, d.Package.LatestVersion)
	}
	if !installsource.IsContentDigest(v.ContentHash) {
		return *v, fmt.Errorf("%w: %s@%s", ErrUnpinned, d.Package.Slug, v.Version)
	}
	switch d.Package.Kind {
	case "plugin":
		if !commitPinned(v.Source) {
			return *v, fmt.Errorf("%w: plugin source %q is not pinned to a commit", ErrBadSource, v.Source)
		}
	case "skill":
		if !remoteSource(v.Source) {
			return *v, fmt.Errorf("%w: %q", ErrBadSource, v.Source)
		}
	case "mcp":
		if !remoteSource(v.Source) && !npmPackage(v.Source) {
			return *v, fmt.Errorf("%w: %q", ErrBadSource, v.Source)
		}
	default:
		return *v, fmt.Errorf("%w: kind %q", ErrBadSource, d.Package.Kind)
	}
	return *v, nil
}

// remoteSource admits an https URL or the git:github.com/ shorthand. A local
// path would resolve on this machine, which no reviewer has seen.
func remoteSource(s string) bool {
	if rest, ok := strings.CutPrefix(s, "git:github.com/"); ok {
		s = "https://github.com/" + rest
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

// commitPinned admits a GitHub tree URL whose ref is a full commit. A branch
// ref would resolve its subpath at whatever the branch holds at apply time.
func commitPinned(s string) bool {
	if rest, ok := strings.CutPrefix(s, "git:github.com/"); ok {
		s = "https://github.com/" + rest
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.User != nil || u.RawQuery != "" {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return len(parts) >= 4 && parts[2] == "tree" && fullCommit.MatchString(parts[3])
}

var fullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

func npmPackage(s string) bool {
	name := strings.TrimPrefix(s, "@")
	if name == "" || strings.ContainsAny(s, `\:`) || strings.HasPrefix(s, ".") {
		return false
	}
	parts := strings.Split(name, "/")
	if len(parts) > 2 || (len(parts) == 2 && !strings.HasPrefix(s, "@")) {
		return false
	}
	for _, p := range parts {
		if !slugPart(p) {
			return false
		}
	}
	return true
}

func doneItems(raw json.RawMessage) []Installed {
	var actions []struct {
		Kind       string `json:"kind"`
		Name       string `json:"name"`
		Status     string `json:"status"`
		Target     string `json:"target"`
		ConfigPath string `json:"configPath"`
	}
	if json.Unmarshal(raw, &actions) != nil {
		return nil
	}
	var out []Installed
	for _, a := range actions {
		if a.Status != "done" {
			continue
		}
		target := a.Target
		if a.Kind == "mcp" {
			target = a.ConfigPath
		}
		out = append(out, Installed{Kind: a.Kind, Name: a.Name, Target: target})
	}
	return out
}

// NewInstallSource is the installer a host gives Service: install_source with
// the approved-plan ticket required.
func NewInstallSource(projectRoot string, hc *http.Client, onDisconnect installsource.OnDisconnectFunc) func() *installsource.Tool {
	return func() *installsource.Tool {
		return installsource.NewTool(installsource.Options{
			ProjectRoot:         projectRoot,
			HTTPClient:          hc,
			OnDisconnect:        onDisconnect,
			RequireApprovedPlan: true,
		})
	}
}
