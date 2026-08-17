# Reasonix Studio (Wails shell)

A native desktop window around the Reasonix Go kernel. The window hosts a React
SPA; every request the SPA makes is routed in-process to the kernel's own HTTP
server (`internal/serve`) instead of to a set of bound Go methods, so the
desktop, `reasonix serve`, and the remote UI all answer the same routes.

```
┌──────────────────────────────────────────────────────────────┐
│  webview (React + TS, Vite)  —  desktop/frontend-next         │
│    fetch("/api/…") · EventSource("/api/events")               │
└───────────────▲────────────────────────────┬─────────────────┘
                │                            │
┌───────────────┴────────────────────────────▼─────────────────┐
│  desktop/next  main.go                                        │
│    isAPIPath / isHubPath ──▶ serve.Server (kernel HTTP mux)   │
│    everything else       ──▶ frontendAssets() (the SPA)       │
└───────────────▲────────────────────────────┬─────────────────┘
                │                            │
┌───────────────┴────────────────────────────▼─────────────────┐
│  internal/boot.Build → internal/control.Controller (kernel)   │
│  (same assembly the CLI uses: providers, tools, gate, …)      │
└───────────────────────────────────────────────────────────────┘
```

Because the SPA speaks HTTP rather than `window.go.*`, `desktop/next` stays
thin: it owns the window, the platform glue (dock icon, file drop, window fit),
and updates. Anything a frontend needs is a kernel route, and
`next/route_parity_test.go` fails if the shell routes one of them to the assets.

## Layout

| Path | What it is |
| --- | --- |
| `next/` | the Wails shell: window, routing, updates, platform glue |
| `frontend-next/` | the SPA it serves |
| `internal/update/` | manifest, download, verify, apply — shared by the shell and the helper |
| `cmd/update-helper/` | the elevated half of an update (dpkg on Linux, versioned install on Windows) |
| `cmd/studio-manifest/`, `cmd/sign/`, `cmd/windows-resource/` | release-side tools |
| `third_party/go-webview2/` | vendored fork; see below |

## Why a nested module

`desktop/` is its own Go module (`module reasonix/desktop`, `replace reasonix =>
../`). That keeps the CGO + WebKit build entirely separate from the CLI's
`CGO_ENABLED=0` single-static-binary guarantee: the parent module's `go build /
vet / test ./...` skip this directory, while the import path stays under
`reasonix/` so it can still import the `reasonix/internal/*` kernel.

## The vendored WebView2 fork

`replace github.com/wailsapp/go-webview2 => ./third_party/go-webview2` carries
patches upstream does not have: mixed-DPI monitor-scale detection (#5862),
`--no-proxy-server` so the loopback UI ignores stale system proxies, and native
renderer-failure recovery. `next/webview2_patch_test.go` reads the fork's AST
and fails if a Wails bump silently drops any of them.

## Prerequisites

- Go (matches the parent module).
- Node 24+ and **pnpm 10** (`npm install -g pnpm@10`).
- Platform webview libs: macOS ships WebKit; Windows needs the Edge **WebView2**
  runtime; Linux needs `libgtk-3-dev` and `libwebkit2gtk-4.1-dev` (4.0 is gone
  from Ubuntu 24.04+ and Fedora 40+, so Linux builds always carry the
  `webkit2_41` tag).

No Wails CLI is required — `scripts/studio-build.sh` drives `go build` directly.

## Running it

```sh
make studio          # build the SPA + shell and launch (macOS: inside an .app bundle)
```

On macOS that bundle is not optional: LaunchServices only treats a bundled
process as a real GUI app, and a bare binary makes native panels — "Add a
folder…" — open and close in the same beat.

For frontend iteration, run the kernel and point Vite at it:

```sh
go run ./cmd/reasonix serve                       # from the repository root
cd desktop/frontend-next && pnpm install && pnpm dev   # :5273, proxies API paths
```

## Building and testing

```sh
bash scripts/studio-build.sh --shell-only         # compile the shell for this host
bash scripts/studio-build.sh darwin/arm64 v0.1.0  # full artifact into dist/

make studio-test                                  # go test ./... for this module
cd frontend-next && pnpm typecheck && pnpm build
```

Linux additionally builds a `.deb`; it is what makes Studio self-updating there,
because the shared apply path stages single files and Studio ships a binary plus
an SPA tree. See `docs/STUDIO_RELEASE.md`.
