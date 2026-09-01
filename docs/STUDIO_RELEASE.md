# Releasing Reasonix Studio

Studio ships from the `studio` branch and publishes as a prerelease: it does not
enter the website's download page, does not claim the repository-wide "latest"
release, and does not touch the desktop line's rollback catalog.

## Its own line

`release-studio.yml` is the only release workflow this branch carries. It has its
own tag namespace (`studio-v*`), its own environment gate and its own rollback
catalog, so nothing it publishes can reach the desktop line's users.

Windows signing is the one boundary that still bites. The production SignPath
policy allows exactly one origin (protected `main-v2`), which is what lets it
avoid trusting wildcard tag-like branch names. A build from `studio` cannot
request it, so **Windows artifacts are unsigned** and users see a SmartScreen
warning. macOS is not affected: one Developer ID signs any bundle identifier its
team owns, so Studio is signed and notarized exactly as the desktop line is.

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
| `APPLE_CERT_P12`, `APPLE_CERT_PASSWORD`, `APPLE_API_KEY_P8`, `APPLE_API_KEY_ID`, `APPLE_API_ISSUER_ID` | Developer ID signing and notarization |

The minisign key is deliberately the desktop line's: the public key that verifies
it is compiled into the client (`update.PublicKey`), so a separate key would not
verify. The five `APPLE_*` secrets are required rather than optional — the macOS
job fails closed without them, because an unsigned build that reached users would
break the next self-update rather than the first launch. If the R2 secrets are
absent the GitHub release still publishes, but the catalog is not updated and
running builds will not see the new version.

## Cutting a release

Write the version's notes first. `release-studio.yml` reads
`release-notes/studio/<semver>.md` — named without the tag's v — and appends
the standing text about signing,
installation and self-update; without that file it publishes the standing text
alone and warns.

```bash
$EDITOR release-notes/studio/2.0.0.md
git checkout studio
git tag studio-v2.0.0
git push origin studio-v2.0.0
```

The tag triggers `release-studio.yml`, which:

1. validates the tag shape (`studio-vMAJOR.MINOR.PATCH[-PRERELEASE]`) and that
   it sits on `studio` history;
2. builds Windows x64, macOS universal and Linux x64 with electron-builder,
   from `desktop/electron/electron-builder.yml`. Windows and macOS package in
   two steps because both need something done to the bundle in between — an
   Authenticode signature there, notarization and stapling here;
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
  studio-v2.0.0/…            # Studio artifacts, latest.json, signatures
  desktop-v1.26.0/…          # desktop artifacts
```

Tag prefixes cannot collide because the tag names differ, so Studio artifacts sit
beside desktop ones without a separate bucket.

## Self-update

`latest.json` carries three maps and the updater reads each by role:

- `downloads` is what a person is offered, so it lists only what installs itself:
  the `.dmg`, the Windows installer, the `.deb`.
- `platforms` is what the updater resolves — the Windows installer, which it runs
  to upgrade in place, and the macOS universal `.zip`, whose bundle it swaps. A
  portable archive is never listed: resolving one would hand the updater an
  artifact it cannot install.
- `nativePackages` carries the `.deb`, which is how Linux upgrades, because dpkg
  replaces the binary and the SPA tree together.

`desktop/cmd/studio-manifest` derives all three from the artifact names, and its
tests assert the manifest never claims a platform it cannot install.

The Windows installer is started as the user who asked for the update, not
elevated. It requests nothing in its manifest and raises consent itself only if
that user chooses an all-users install; starting it with `runas` would put it in
the consenting account, where its per-user default writes that profile and
leaves the updating user on the build they had. `update.StudioLine` states which
of the two it ships, and a line that states neither is refused rather than
guessed for.

That installer also takes over a Wails-era install rather than sitting beside
it. The old one is per-machine under Program Files with its own uninstall key,
this one is per-user under a key derived from the appId, so nothing about
installing it would have replaced the old one — both would have stayed in
Add/Remove Programs and only one would ever have been updated again.
`desktop/electron/assets/installer.nsh` runs the old uninstaller from
`customInstall`; a declined consent prompt leaves both installed rather than
failing the install.

## Known gaps

- No `versioninfo` stamp and **no windows/arm64 build**: `desktop/next` builds
  with plain `go build` rather than the Wails CLI, and Windows x64 takes its icon
  from the committed `rsrc_windows_amd64.syso` — arm64 has no `.syso`.
- Giving `desktop/next` its own `wails.json` fixes the version stamp and arm64
  together. When doing it, keep Studio's install directory and uninstall registry
  key distinct from `reasonix-desktop` — sharing them would let installing Studio
  overwrite an existing desktop install.
- Installs of the desktop line (`desktop-v*`) read the root catalog, so they are
  never offered Studio. Migrating them is not wired up.
