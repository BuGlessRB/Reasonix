# Studio: the Electron migration contract

This is not a plan and carries no dates. It is the set of boundaries the
migration from the Wails shell to the Electron shell runs on, each one written
down only after code demonstrated it. Use it in review by asking one question:
**which rule does this change cross?**

Two shells run the same `frontend-next` build against the same Go kernel today.
That is the state these rules protect.

## 1. Wails is frozen

The Wails shell (`desktop/next`) takes blocker fixes and security fixes. It does
not take new capabilities, new compatibility layers, or further WebView patches.

New shell capability lands on Electron. Where a capability has to exist in both
during the crossing, the **old shell adapts to the new protocol**, never the
other way round.

## 2. Where a thing belongs

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

## 3. The loopback boundary

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

## 4. One transport

Ordinary business runs over HTTP and SSE, the same surface a browser gets. No
IPC variant of the agent API is built for Electron. Main-process IPC carries OS
capability only, and its handlers verify the sender.

The shell's main process may reach the kernel directly over that same HTTP
(`desktop/electron/src/hostclient.js`) rather than through the renderer: a
surface that asked the page for its state would put the credential within reach
of the page, and would go blank exactly when the window is hidden.

The static page is served under one namespace, `/_studio/`, and every other
path belongs to the kernel. The inverse — a list of the kernel's routes with
everything else falling through to the page — is the arrangement the Wails
asset server forced, and it has to be edited every time the kernel grows an
endpoint.

## 5. State taxonomy

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

## 6. Retirement is atomic

While Wails is a supported shell, nothing that keeps it working may be deleted
ahead of it. These go in one transaction or not at all:

`desktop/next` · the local event pump and its lifecycle · `remote_pump.go` ·
`/rx-replay` · every `WAILS_*` constant in the frontend · the `apiPaths` /
`apiPrefixes` whitelist and its route-parity test · `third_party/go-webview2` ·
the Wails dependency, its CGO glue and the GTK/WebKitGTK build requirements.

Deleting any one of these first leaves a broken Wails rather than a smaller
tree. This rule already caught one attempt: `remote_pump.go` looked redundant
because the hub's reverse proxy streams the same frames, but the page inside
that shell reads the bus for **every** pane, so the proxy's stream is what a
client reaching `/rt/<id>/events` over real HTTP gets and the page there never
asks. Removing it would have taken SSH workspaces down.

## 7. What allows a retirement

Not "Electron has something similar". The condition is **Wails has no exclusive
semantics left**, per capability, demonstrated rather than argued. Spelled out,
because a near miss of it has already happened once:

> Wails may retire only when every capability it holds exclusively either has an
> Electron implementation consuming the canonical protocol or state, or has been
> deliberately dropped as a product capability. **A protocol existing is not
> parity.** Making a capability canonical moves it out of one shell; it does not
> put it into the other.

And:

- The behaviour is reproduced, not merely the API.
- It is verified at the layer where the old one broke. Editing shortcuts were
  accepted by driving real OS key events, because `MenuItem.click()` runs a JS
  handler a role does not have and `sendInputEvent` lands below the dispatch
  that consults the menu — a template containing the right words is what passed
  last time.
- Where a claim cannot be verified, it is recorded as unverified rather than
  rounded up.

## 8. Not yet equivalent

Two ledgers, because they answer different questions. The first decides whether
Wails may retire at all. The second does not.

An item leaves either list when its behaviour is reproduced and checked, not
when something resembling it exists.

### Functional parity — these block retirement

- **Remote workspaces in Electron.** `cmd/reasonix-studio-host` supplies no
  `Remote` to its hub, and `newRemoteLink` / `fetchRemoteBinary` are still owned
  by `desktop/next`. So attach, browse and probe are Wails-only, and the shell
  that has them is the one being retired.

  Read this next to the entry it is easily confused with: the question a dial
  stops for is canonical now — one broker, one snapshot, one answer endpoint,
  and no shell owns it. That made the *semantics* shared. It did not give
  Electron a remote workspace, because nothing there dials.

- **Single instance.** Wails locks per canonicalized `REASONIX_HOME`, so two
  homes coexist and one home never opens twice. Electron's own lock has no such
  key, and the identity is Reasonix's rather than the shell's.

- **Updater.** The macOS handoff re-executes the running binary and waits on its
  own PID. Under Electron the owner, the executable and the wait condition are
  three different processes.

- **Version pinning** and the **folder picker.** Still answered by Wails
  bindings; every other client answers null.

### Presentation parity — these do not

- **Status icon glyph.** The mood mark is drawn in Go inside the Wails shell.
  Electron shows a fixed icon while the fold itself — the sentence, the menu
  words, the counts — comes from the kernel and is checked against it. What is
  missing is the colour, not the state.

- **Window bounds.** Electron clamps to the display it opens on but does not yet
  remember where it was.
