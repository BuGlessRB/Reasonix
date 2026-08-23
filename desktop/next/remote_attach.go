package main

import (
	"context"
	"os"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/netclient"
	"reasonix/internal/releaseasset"
	"reasonix/internal/remote/attach"
	"reasonix/internal/serve"
)

// remoteAttacher gives the hub a way to reach a workspace on another machine.
// Prompts are left unset for now: a first-seen host key and a typed passphrase
// both need a dialog this window does not have yet, and refusing is the safe
// direction — a host already in known_hosts, reached by key or agent, connects.
func remoteAttacher(ctx context.Context) func(context.Context, string, string) (serve.RemoteEndpoint, func(), error) {
	pool := attach.NewPool(ctx, attach.Options{
		LocalBinary: currentExecutable(),
		Version:     version,
		FetchBinary: fetchRemoteBinary,
	})
	return func(ctx context.Context, host, workspace string) (serve.RemoteEndpoint, func(), error) {
		ep, err := pool.Attach(ctx, host, workspace, attach.Call{})
		if err != nil {
			return serve.RemoteEndpoint{}, nil, err
		}
		return serve.RemoteEndpoint{
			Host:      ep.Host,
			Workspace: ep.Workspace,
			Addr:      ep.Addr,
			Token:     ep.Token,
		}, ep.Release, nil
	}
}

func currentExecutable() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return ""
}

// fetchRemoteBinary downloads the release for a remote's platform when the host
// has no reasonix and cannot install one itself.
func fetchRemoteBinary(ctx context.Context, version, goos, goarch string) ([]byte, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	client, err := netclient.NewHTTPClient(cfg.NetworkProxySpec(), netclient.TransportOptions{
		ResponseHeaderTimeout: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	client.Timeout = 2 * time.Minute
	return releaseasset.DownloadCLI(ctx, client, version, goos, goarch)
}
