---
owner: @SivanCola
backup: @esengine
status: active
reviewed: 2026-10-01
---

# Full Sidecar

This is the reference Extension Protocol v2 example. It declares interceptors,
a system-prompt strategy, a fixed-output provider, and structured UI. Use the
[starter extension](../starterextension/README.md) for a smaller first plugin;
use this example to explore the additional host surfaces.

## Build and install

From `sdk/go/examples/fullsidecar` in a source checkout, on macOS or Linux:

```sh
go build -o bin/full-sidecar .
plugin_root="$(pwd -P)"
reasonix plugin install "$plugin_root" --dry-run
reasonix plugin install "$plugin_root" --yes
reasonix plugin doctor full-sidecar
```

From the same directory in PowerShell on Windows:

```powershell
go build -o bin/full-sidecar.exe .
$pluginRoot = (Resolve-Path .).Path
reasonix plugin install $pluginRoot --dry-run
reasonix plugin install $pluginRoot --yes
reasonix plugin doctor full-sidecar
```

Review the dry-run's `FULL TRUST` block before installing: the sidecar executes
outside the Reasonix sandbox and owns the `system_prompt` strategy slot. The
commands above copy the package into Reasonix's global plugin inventory.

Rebuilding the source does not update that copy; reinstall with
`--replace --yes`, then reload. For local iteration, use
`--link --replace --yes`; a linked installation also trusts future changes in
the source directory.

Start a new session, or run `/reload` while the current session is idle. Keep
your normal configured model selected for the following checks.

## Observe the installed contributions

Send an ordinary prompt to start a turn. The extension publishes a status
label, `fullsidecar online`, and a card titled `fullsidecar`. The card declares
the `Run demo` action. In Studio, use that action to open the form; frontends
that support extension slash actions expose it as `/full-sidecar:demo`.

Enter a name and choose whether to shout the greeting. The host routes the
answers back to the sidecar, which publishes `Hello, <name>!` as a notification
(uppercase when shouting is selected). Dismissing the form cancels this demo
without producing a greeting.

The strategy wraps the model's system prompt with:

```text
You are Reasonix running under the fullsidecar demo strategy.
```

The host injects the installed plugin name, `full-sidecar`, so the provider
appears in the merged catalog as `plugin/full-sidecar/fake/echo`. It always
streams `fake-hello fake-world`, a `lookup` tool call, and fixed usage numbers.

It ignores the task and is a protocol fixture, not a model for normal work.
The automated host check below uses a separate recording provider to verify
the strategy without interpreting this fixture's output as task quality.

The `/fs ` prefix in `main.go` is a raw `input.receive` protocol fixture. It
is not a registered slash command.

The host composes turn context before
calling that interceptor, so the callback's `text` need not begin with the
user's text; frontends can also refuse unknown slash commands before a turn
starts.

Use the demo action and the host checks below for installation
verification, rather than treating `/fs hello` in a composer as that check.

Likewise, the tool interception examples exercise protocol decisions: they
block a tool named `dangerous_exec` and add a `sandbox` argument to a tool
named `read`. These example names and arguments do not grant or enforce host
sandbox authority.

## Disable, restore, and remove

```sh
reasonix plugin disable full-sidecar
reasonix plugin enable full-sidecar
reasonix plugin remove full-sidecar --yes
```

After each change, start a new session or reload while idle. Disabling or
removing the package removes its strategy, provider entry, and actions from
the new runtime.

A disabled package remains installed and can be enabled
again. Removing a copied package deletes the installed copy; removing a
linked package leaves its source directory in place.

## Run the host checks

From the repository root:

```sh
go test ./internal/assembly/boot/ -run '^TestEffectFullsidecarInstalledHostSurfaces$' -count=1
go test ./internal/ext/extension/conformance/ -count=1
```

The boot effect test builds this actual SDK example without fetching modules,
previews and applies a copy installation in an isolated home, removes the
source, and drives the real boot assembly across absent, installed, disabled,
reenabled, and removed states.

It asserts the strategy at the provider
request, the provider descriptor in the merged catalog, and the status,
card, blocking form, and answered greeting at the controller's frontend
event sink. It also waits for each started sidecar to exit on close.

Conformance separately checks raw input and tool interception, provider
streaming and cancellation, protocol validation, and shutdown. Neither suite
proves a live model's task quality or a browser's rendering of the surfaces.
