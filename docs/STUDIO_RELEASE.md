# Releasing Reasonix Studio

Studio ships from the `studio` branch, in parallel with `main-v2`. It is a
preview line: it does not enter the website's download page, does not claim the
repository-wide "latest" release, and does not touch the desktop line's rollback
catalog. Nothing here changes how `main-v2` publishes.

## Why it cannot use the stable pipeline

`release-stable.yml` validates that the CLI, npm and desktop tags all point at a
reviewed candidate on **main-v2 history**. Studio commits are never on that
history, so they cannot pass its preflight — hence a separate line rather than a
new input to the existing one.

The same boundary applies to signing. The production SignPath policy allows
exactly one origin (protected `main-v2`), which is what lets it avoid trusting
wildcard tag-like branch names. A build from `studio` cannot request it, so
**Windows artifacts are unsigned** and users see a SmartScreen warning. macOS
artifacts are ad-hoc signed, not notarized: users clear the quarantine attribute
with `xattr -dr com.apple.quarantine`.

## One-time setup

`release-studio.yml` gates publication on a GitHub environment named
`studio-release`. Create it with the owner as a required reviewer:

- Settings → Environments → New environment → `studio-release`
- Required reviewers: the owner

It reuses these existing repository secrets — none are Studio-specific:

| Secret | Used for |
| --- | --- |
| `MINISIGN_PRIVATE_KEY`, `MINISIGN_PASSWORD` | detached signatures |
| `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_ACCOUNT_ID`, `R2_BUCKET` | catalog + artifact mirror |

The minisign key is deliberately the desktop line's: the public key that verifies
it is compiled into the client (`update.PublicKey`), so a separate key would not
verify. If R2 secrets are absent the GitHub release still publishes, but the
catalog is not updated and running builds will not see the new version.

## Cutting a release

```bash
git checkout studio
git tag studio-v0.1.0
git push origin studio-v0.1.0
```

The tag triggers `release-studio.yml`, which:

1. validates the tag shape (`studio-vMAJOR.MINOR.PATCH[-PRERELEASE]`) and that
   it sits on `studio` history;
2. builds Windows x64, macOS universal and Linux x64 via
   `scripts/studio-build.sh`;
3. waits for the `studio-release` approval;
4. minisign-signs every artifact and verifies each signature;
5. generates `latest.json` with `desktop/cmd/studio-manifest`;
6. publishes a GitHub **prerelease** (never `latest`);
7. mirrors artifacts to `dl.reasonix.io/<tag>/` and updates
   `dl.reasonix.io/studio/versions.json`.

`workflow_dispatch` with an existing tag recovers a failed run.

## R2 layout

```
dl.reasonix.io/
  versions.json              # desktop line — release-studio.yml never writes this
  studio/versions.json       # Studio's catalog; studioCatalog in desktop/next
  studio-v0.1.0/…            # Studio artifacts, latest.json, signatures
  desktop-v1.26.0/…          # desktop artifacts
```

Tag prefixes cannot collide because the tag names differ, so Studio artifacts sit
beside desktop ones without a separate bucket.

## Self-update is not available yet

The version panel reads Studio's catalog and will show that a newer version
exists, but installing it is not wired up, so `latest.json` lists artifacts under
`downloads` and leaves `platforms` empty. Asking for a version then reports that
no installable package exists for this platform and points at the download page.

That is a property of the shared install path, not an oversight:

- **Windows** installs through the NSIS updater (`installerCommand` passes `/D=`),
  and Studio has no `wails.json`, so it produces no installer.
- **Linux** reads fixed member names out of the tarball —
  `update.ExtractReleaseUnit` requires `reasonix-desktop`, `reasonix-guard` and
  `reasonix` — which Studio's archive does not contain.
- **macOS** self-update additionally requires a Developer ID signature and
  notarization (`update.MacSelfUpdate`), which this line cannot obtain.

Turning self-update on means teaching the apply path Studio's layout and filling
`platforms` in the same change. `desktop/cmd/studio-manifest`'s tests assert that
the manifest claims nothing it cannot install, so the claim and the capability
have to land together.

## Known gaps

- No Windows installer and no `versioninfo` stamp: `desktop/next` builds with
  plain `go build`, not the Wails CLI. Windows x64 gets its icon from the
  committed `rsrc_windows_amd64.syso`; **arm64 has no `.syso`, so it is not built**.
- Giving `desktop/next` its own `wails.json` fixes the installer, the version
  stamp and arm64 at once, and is the prerequisite for self-update. When doing
  it, keep Studio's install directory and uninstall registry key distinct from
  `reasonix-desktop` — sharing them would let installing Studio overwrite an
  existing desktop install.
