// Package attach turns a configured SSH host into a reachable kernel: it dials
// the connection, makes sure a `reasonix serve` is running for a workspace on
// the other side, and forwards it to a local address a frontend can call.
//
// One connection carries every workspace opened on that host, and one forward
// carries every pane opened on that workspace, so both are reference-counted
// and the last holder out tears them down. The remote serve is left running:
// it outlives the link by design, and the next connect reuses it.
package attach

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"reasonix/internal/config"
	"reasonix/internal/remote"
	"reasonix/internal/remote/bootstrap"
	"reasonix/internal/remote/forward"
)

// Options is what every attach on this pool shares: who answers a credential
// prompt, and where a remote install comes from when the host has no reasonix.
type Options struct {
	Prompts     Prompts
	Install     string // auto|npm|upload|never; empty => the host entry's own
	LocalBinary string // this process's binary, for a same-platform upload
	Version     string // this release, for a verified cross-platform download
	FetchBinary func(ctx context.Context, version, goos, goarch string) ([]byte, error)
	// Dial builds a host's connection. Nil resolves it from configuration —
	// which is the only part of an attach that needs a configured machine, so
	// substituting it is what lets the rest be exercised against a real one.
	Dial func(host string, prompts Prompts) (*remote.Client, error)
}

// Call is what one attach decides for itself: where the progress of a first
// connect is reported, and who watches the link's state afterwards.
type Call struct {
	Progress func(step, detail string)
	OnStatus func(remote.StatusEvent)
}

// Endpoint is one remote workspace, reachable over a local address until it is
// released. Token is what the remote kernel's gate expects.
type Endpoint struct {
	Host      string
	Workspace string
	Addr      string
	Token     string

	once    sync.Once
	release func()
}

// Release drops this holder's claim on the workspace and its connection. Safe
// to call more than once — a pane closing twice must not free a live link.
func (e *Endpoint) Release() {
	if e == nil {
		return
	}
	e.once.Do(func() {
		if e.release != nil {
			e.release()
		}
	})
}

// Pool keeps one supervised connection per host and hands out endpoints riding
// it. Its context outlives any single attach: a supervisor bound to the caller
// that happened to dial first would die when that pane closed.
type Pool struct {
	ctx  context.Context
	opts Options

	mu    sync.Mutex
	links map[string]*link
}

func NewPool(ctx context.Context, opts Options) *Pool {
	return &Pool{ctx: ctx, opts: opts, links: map[string]*link{}}
}

// gate is a value several callers wait on while the first one produces it.
type gate struct {
	ready chan struct{}
	err   error
}

func (g *gate) wait(ctx context.Context) error {
	select {
	case <-g.ready:
		return g.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

type link struct {
	gate
	host    string
	client  *remote.Client
	install string // the host entry's own strategy, read once at dial
	refs    int
	spaces  map[string]*space
}

type space struct {
	gate
	// key is what spaces is keyed by — what the caller asked for. The remote
	// kernel resolves ~ and answers with an absolute path, so the two part ways
	// and deleting by the resolved one would leave the entry behind.
	key       string
	workspace string
	addr      string
	token     string
	forward   string
	refs      int
}

// Attach makes host's workspace reachable locally. ctx bounds this attach, but
// not the connection it may create: an established link belongs to the pool.
// A dial already in flight is not interruptible — DialTimeout bounds that one.
func (p *Pool) Attach(ctx context.Context, host, workspace string, call Call) (*Endpoint, error) {
	host, workspace = strings.TrimSpace(host), strings.TrimSpace(workspace)
	if host == "" {
		return nil, errors.New("attach: no host named")
	}
	l, dialer := p.holdLink(host)
	var unsubscribe func()
	if dialer {
		unsubscribe = p.dial(l, call)
	}
	if err := l.wait(ctx); err != nil {
		p.dropLink(l)
		return nil, err
	}
	if !dialer && call.OnStatus != nil {
		// Only a late caller subscribes here. The one that dialed did so before
		// Start, or it would have missed the connect it was waiting on.
		unsubscribe = l.client.Subscribe(call.OnStatus)
	}
	s, server := p.holdSpace(l, workspace)
	if server {
		p.serve(ctx, l, s, workspace, call)
	}
	if err := s.wait(ctx); err != nil {
		p.dropSpace(l, s, unsubscribe)
		return nil, err
	}
	return &Endpoint{
		Host:      host,
		Workspace: s.workspace,
		Addr:      s.addr,
		Token:     s.token,
		release:   func() { p.dropSpace(l, s, unsubscribe) },
	}, nil
}

// Close tears down every connection this pool holds.
func (p *Pool) Close() {
	p.mu.Lock()
	links := make([]*link, 0, len(p.links))
	for _, l := range p.links {
		links = append(links, l)
	}
	p.links = map[string]*link{}
	p.mu.Unlock()
	for _, l := range links {
		if l.client != nil {
			_ = l.client.Close()
		}
	}
}

// holdLink takes a reference on host's connection, creating the entry if this
// is the first. The second return says whether this caller must produce it.
func (p *Pool) holdLink(host string) (*link, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if l := p.links[host]; l != nil {
		l.refs++
		return l, false
	}
	l := &link{
		gate:   gate{ready: make(chan struct{})},
		host:   host,
		refs:   1,
		spaces: map[string]*space{},
	}
	p.links[host] = l
	return l, true
}

// dial produces the connection every waiter on this link is blocked for, and
// returns the caller's own status subscription.
func (p *Pool) dial(l *link, call Call) (unsubscribe func()) {
	fail := func(err error) {
		l.err = err
		// Dropped from the table before the waiters wake: a cached failure would
		// answer every later attach with an error nobody can retry past.
		p.forgetLink(l)
	}
	defer close(l.ready)
	cfg, err := config.Load()
	if err != nil {
		fail(err)
		return nil
	}
	if entry, ok := cfg.RemoteHost(l.host); ok {
		l.install = entry.ServeInstallMode()
	}
	build := p.opts.Dial
	if build == nil {
		build = func(host string, prompts Prompts) (*remote.Client, error) { return Dial(cfg, host, prompts) }
	}
	client, err := build(l.host, p.opts.Prompts)
	if err != nil {
		fail(err)
		return nil
	}
	if call.OnStatus != nil {
		unsubscribe = client.Subscribe(call.OnStatus)
	}
	if err := client.Start(p.ctx); err != nil {
		_ = client.Close()
		fail(err)
		return unsubscribe
	}
	l.client = client
	return unsubscribe
}

func (p *Pool) forgetLink(l *link) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.links[l.host] == l {
		delete(p.links, l.host)
	}
}

func (p *Pool) dropLink(l *link) {
	p.mu.Lock()
	l.refs--
	last := l.refs <= 0
	if last && p.links[l.host] == l {
		delete(p.links, l.host)
	}
	client := l.client
	p.mu.Unlock()
	if last && client != nil {
		_ = client.Close()
	}
}

func (p *Pool) holdSpace(l *link, workspace string) (*space, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s := l.spaces[workspace]; s != nil {
		s.refs++
		return s, false
	}
	s := &space{
		gate:      gate{ready: make(chan struct{})},
		key:       workspace,
		workspace: workspace,
		refs:      1,
	}
	l.spaces[workspace] = s
	return s, true
}

// serve brings up the remote kernel for one workspace and binds the forward
// that reaches it.
func (p *Pool) serve(ctx context.Context, l *link, s *space, workspace string, call Call) {
	defer close(s.ready)
	install := p.opts.Install
	if install == "" {
		install = l.install
	}
	res, err := bootstrap.EnsureServe(ctx, l.client, bootstrap.Options{
		Workspace:      workspace,
		Install:        install,
		LocalBinary:    p.opts.LocalBinary,
		LocalGOOS:      runtime.GOOS,
		LocalGOARCH:    runtime.GOARCH,
		ProductVersion: p.opts.Version,
		FetchBinary:    p.opts.FetchBinary,
		MinVersion:     bootstrap.MinServeVersion,
		Progress:       call.Progress,
	})
	if err != nil {
		s.err = err
		p.forgetSpace(l, s)
		return
	}
	// Named per workspace: a second one on this host must not replace the
	// first one's forward, which one shared reserved name would do.
	name := "serve:" + res.State.Workspace
	bound, err := l.client.Forwards().Add(forward.Spec{
		Name:       name,
		Direction:  forward.Local,
		BindAddr:   "127.0.0.1:0",
		TargetAddr: res.State.Addr,
	})
	if err != nil {
		s.err = fmt.Errorf("forward remote serve: %w", err)
		p.forgetSpace(l, s)
		return
	}
	s.workspace, s.addr, s.token, s.forward = res.State.Workspace, bound, res.Token, name
}

func (p *Pool) forgetSpace(l *link, s *space) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if l.spaces[s.key] == s {
		delete(l.spaces, s.key)
	}
}

// dropSpace releases one holder's claim. Every attach took a reference on both
// the workspace and the link, so every release gives back both.
func (p *Pool) dropSpace(l *link, s *space, unsubscribe func()) {
	if unsubscribe != nil {
		unsubscribe()
	}
	p.mu.Lock()
	s.refs--
	last := s.refs <= 0
	if last && l.spaces[s.key] == s {
		delete(l.spaces, s.key)
	}
	client := l.client
	p.mu.Unlock()
	if last && s.forward != "" && client != nil {
		_ = client.Forwards().Remove(s.forward)
	}
	p.dropLink(l)
}
