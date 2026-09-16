# Studio: the shell's boundaries

This is not a plan and carries no dates. It is the set of boundaries Studio's
shell runs on, each one written down only after code demonstrated it. Use it in
review by asking one question: **which rule does this change cross?**

It began as the contract for the migration off the Wails shell, which is done:
`desktop/next`, the vendored WebView2 fork, the frontend's Wails surface and the
Wails-only kernel entries are gone, and the Electron shell is the only one.
The rules below are what that migration established, and they outlive it.

## 1. Where a thing belongs

- **The renderer** owns presentation. It never holds a credential, never learns
  which shell it is in, and asks `HostPort` (`frontend-next/src/port/host.ts`)
  for anything a page cannot do.
- **The shell's main process** owns OS capability: windows, menus, dialogs, the
  status icon, the platform opener. It owns no business state.
- **The Go kernel** owns durable intent and business state, and is the only
  writer of either.

A shell that keeps its own copy of kernel state is the failure this rule exists
for. It is checked, not assumed: `desktop/electron/test/smoke.js` holds the
tray's displayed fold against a fresh read from the kernel, and giving the tray
a counter of its own turns it red.

## 2. The loopback boundary

The kernel reaches a separate-process renderer through a socket, so the socket
carries a boundary the host owns and no configuration can switch off
(`internal/serve/loopback.go`):

- `tcp4 127.0.0.1:0` — never a wildcard, never a name a resolver answers for.
- A credential minted per launch from `crypto/rand`, never read from config and
  never persisted.
- Cookie only, set by the shell's main process before anything loads: HttpOnly,
  SameSite=Strict, host-only, no expiry. It never reaches the page.
- Exact `Host`, and exact `Origin` on anything that changes state; a read may
  arrive without an origin, a write may not.
- Checks run outermost first, so a caller that reached the socket under another
  name learns nothing about the credential.

The user's `[serve]` authentication does not participate. Studio's boundary is
this gate, and the two cannot share one cookie — a configured `auth_mode =
"token"` left in place would refuse the launch credential on every request.
`cmd/reasonix-studio-host` strips authentication out of the config it hands the
hub for exactly that reason, and a test pins it.

## 3. One transport

Ordinary business runs over HTTP and SSE, the same surface a browser gets. No
IPC variant of the agent API is built for Electron. Main-process IPC carries OS
capability only, and its handlers verify the sender.

The shell's main process may reach the kernel directly over that same HTTP
(`desktop/electron/src/hostclient.js`) rather than through the renderer: a
surface that asked the page for its state would put the credential within reach
of the page, and would go blank exactly when the window is hidden.

The static page is served under one namespace, `/_studio/`, and every other
path belongs to the kernel. The inverse — a list of the kernel's routes with everything else falling
through to the page — has to be edited every time the kernel grows an endpoint.

## 4. State taxonomy

Three kinds, carried three ways. Deciding which one a thing is comes before
deciding its API.

| Kind | Carried by | Why |
| --- | --- | --- |
| Durable intent | canonical read/write endpoint | It outlives the process that set it, and a second copy would give two shells two answers. |
| Projection | pull | Recomputed on demand; losing a read costs nothing the next one does not restore. |
| State whose loss changes execution | replayable event or recoverable snapshot | A client that missed it cannot recover by asking again later. |

The status icon is the worked example of the first two: prefs are durable
intent and live in the config file (`GET`/`PUT /tray/prefs`), while the fold the
icon paints is a projection (`GET /tray/state`) that gets no id, no replay and
no lifecycle. Promoting a projection to an event makes a rendering detail into a
fact the stream has to guarantee.

A pending question put to the user is the third kind, and must not be modelled
as a notification.

## Known gaps

These are presentation, not function, and were carried across the retirement
rather than being fixed by it. An item leaves this list when its behaviour is
reproduced and checked, not when something resembling it exists.

- **Status icon glyph.** The mood mark used to be drawn in Go inside the old
  shell. Electron shows a fixed icon while the fold itself — the sentence, the
  menu words, the counts — comes from the kernel and is checked against it.
  What is missing is the colour, not the state.

- **Window bounds.** Electron clamps to the display it opens on but does not
  remember where it was.

- **Panel title.** macOS ignores a title on an open panel, and the wording
  lives in the page rather than in the shell, so the picker passes none.

- **Crash capture.** `cmd/reasonix-studio-host` installs no recover, so a Go
  panic there writes no crash report. The reports that exist have been fatal
  signals inside the renderer, which no recover reaches either.
