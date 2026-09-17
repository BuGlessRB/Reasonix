---
owner: @esengine
backup: @SivanCola
status: active
reviewed: 2026-09-17
---

# Studio release runbook

Workflow: `.github/workflows/release-studio.yml`. Trigger: push of a `studio-vX.Y.Z` tag.

## 1. Owners

| Role | Person | Responsibility |
| --- | --- | --- |
| Release owner | @esengine | Decides the version, writes the notes, pushes the tag, verifies, recovers. |
| Backup | @SivanCola | Runs this runbook when the release owner is unavailable. |

## 2. Preconditions

| ID | Check | Command |
| --- | --- | --- |
| P1 | The release commit is the fetched head of `origin/studio`. | `git fetch origin studio && git rev-parse origin/studio` |
| P2 | Every job of `CI` and `Studio` succeeded on that commit. A flaky job is rerun, not ignored. | `gh run list --branch studio --commit <sha> --json name,conclusion` |
| P3 | `release-notes/studio/X.Y.Z.md` is in that commit and passes `make check`. | `git cat-file -e <sha>:release-notes/studio/X.Y.Z.md` |
| P4 | The version is valid semver and above the latest tag. | `git tag -l 'studio-v*' --sort=-v:refname \| head -1` |
| P5 | The Windows signing mode is known. `true` signs through SignPath; `false` ships unsigned and the body says so. | `gh variable list \| grep STUDIO_SIGNING_ENABLED` |

## 3. Steps

1. Write `release-notes/studio/X.Y.Z.md` (format in section 6), commit, push to `studio`.
2. Wait until P2 holds for the commit that contains the notes.
3. Tag the verified commit and push the tag:

   ```bash
   git fetch origin studio
   SHA=$(git rev-parse origin/studio)
   git tag studio-vX.Y.Z "$SHA"
   git fetch origin studio && test "$(git rev-parse origin/studio)" = "$SHA"
   git push origin studio-vX.Y.Z
   ```

4. Watch the run until it finishes:

   ```bash
   RUN=$(gh run list --workflow release-studio.yml --limit 1 --json databaseId --jq '.[0].databaseId')
   gh run watch "$RUN" --exit-status
   ```

Jobs in the run:

| Job | Does | Gate |
| --- | --- | --- |
| `resolve` | Validates the tag shape and that the commit is on `studio`. | fails on any other ref |
| `signing-contract` | Validates `.signpath/` locally and prints its fingerprint. | does not call SignPath |
| `build` | Builds windows/amd64, darwin/amd64, darwin/arm64, linux/amd64; signs macOS and, if enabled, Windows. | Apple secrets are required |
| `cli` | Builds `reasonix` archives for six OS/arch targets plus `SHA256SUMS`. | fails on a missing archive |
| `publish` | Minisigns, writes `latest.json`, creates the GitHub prerelease, mirrors to R2. | environment `studio-release` |

The `studio-release` environment allows the `studio-v*` tag and the `studio` branch. It has no required reviewer.

## 4. Verification

| ID | Expected | Command |
| --- | --- | --- |
| V1 | Prerelease exists with 22 assets (per-platform packages, `.minisig` files, CLI archives, `latest.json`, `SHA256SUMS`). | `gh release view studio-vX.Y.Z --json isPrerelease,assets --jq '.isPrerelease, (.assets \| length)'` |
| V2 | The catalog lists the new version first. | `curl -s https://dl.reasonix.io/studio/versions.json \| jq -r '.versions[0].tag'` |
| V3 | The manifest is served. | `curl -sI https://dl.reasonix.io/studio-vX.Y.Z/latest.json \| head -1` |
| V4 | The body contains the version notes and the standing install text. | `gh release view studio-vX.Y.Z --json body --jq .body` |

## 5. Recovery

| Symptom | Cause | Action |
| --- | --- | --- |
| No run appears, or a rerun ends in `startup_failure` with no jobs. | GitHub Actions runner outage. | Wait for queued runs to drain, then `gh workflow run release-studio.yml --ref studio -f tag=studio-vX.Y.Z`. |
| A build or publish step failed. | Workflow or runner fault. | Fix on `studio` if needed, then dispatch as above. The dispatch rebuilds from the tag's commit with the workflow from `studio`. |
| Windows signing fails with `Invalid request to SignPath API.` | SignPath rejected the request; the action hides the status. | `gh run rerun <run> --failed --debug` to see it. On quota exhaustion set `STUDIO_SIGNING_ENABLED=false` and dispatch. |
| Signing is restored after an unsigned release. | Artifacts were published unsigned. | Set `STUDIO_SIGNING_ENABLED=true` and dispatch the same tag; `publish` replaces the assets. |
| The body is missing or wrong. | Notes are read from the tag's commit, not the branch. | `gh release edit studio-vX.Y.Z --notes-file <file>`; append the standing text from the previous body. |
| The tag points at the wrong commit and `publish` has not run. | Tagging error. | `git push origin :refs/tags/studio-vX.Y.Z`, delete the local tag, restart at step 3. |
| The tag points at the wrong commit and `publish` has run. | Tagging error after release. | Do not move the tag. Release the next patch version. |

## 6. Release note format

File: `release-notes/studio/X.Y.Z.md`, without the tag's `v`. Language: Chinese. Enforced by `release-note` and `doc-prose`.

| ID | Rule |
| --- | --- |
| R1 | The first line is a one-sentence summary. No other paragraph. |
| R2 | Group headings are `## 新增`, `## 变更`, `## 修复`, `## 移除`, `## 升级须知`, in that order, omitting empty ones. No deeper headings. |
| R3 | One change is one list item within 200 display columns. |
| R4 | Every item names its issue (`#123`), pull request or commit. |
| R5 | Describe what the user observes. The explanation belongs in the linked commit. |

```markdown
本版修复读图模型误报看不到图，并让被 ACL 残留阻塞的 Windows 安装恢复启动。

## 修复

- 读图模型不再声称看不到已附加的图片 (44150b0aa)
- Windows 安装目录带 AppContainer 包 SID 授权时窗口可以正常打开 #10435
```

## 7. Reference

| Name | Kind | Used by |
| --- | --- | --- |
| `APPLE_CERT_P12`, `APPLE_CERT_PASSWORD`, `APPLE_API_KEY_P8`, `APPLE_API_KEY_ID`, `APPLE_API_ISSUER_ID` | secret | macOS signing and notarization |
| `SIGNPATH_API_TOKEN`, `SIGNPATH_ORGANIZATION_ID` | secret | Windows Authenticode signing |
| `STUDIO_SIGNING_ENABLED` | variable | Windows signing switch and the body's Windows sentence |
| `MINISIGN_PRIVATE_KEY`, `MINISIGN_PASSWORD` | secret | detached signatures verified by the updater |
| `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_ACCOUNT_ID`, `R2_BUCKET` | secret | artifact mirror and catalog |

| R2 path | Owner | Content |
| --- | --- | --- |
| `studio/versions.json` | this workflow | Studio catalog, newest first |
| `studio-vX.Y.Z/` | this workflow | artifacts, signatures, `latest.json` |
| `versions.json` | desktop line | never written by this workflow |
