---
owner: @esengine
backup: @SivanCola
status: active
reviewed: 2026-09-25
---

# Reasonix 2.x roadmap

## Purpose

This page tracks what Reasonix 2.x still has to deliver, how it ships, and which decisions are still open.

- Why there are two release lines: the [version roadmap announcement](https://github.com/esengine/DeepSeek-Reasonix/discussions/10748).
- How to move between them: [Moving from 1.x to 2.x](./MIGRATING.md).

## Release lines

| Line | Branch | Status | Ships |
| --- | --- | --- | --- |
| 2.x | `studio` | Active development | Reasonix Studio and the `reasonix` CLI, from `studio-v2.*` tags |
| 1.x | `main-v2` | Maintenance / stable | The 1.x desktop app. The last 1.x CLI on npm and Homebrew is 1.39.0. |

## How 2.x ships

| ID | Rule |
| --- | --- |
| R1 | A `studio-v2.*` tag builds Studio for macOS, Windows and Linux, and `reasonix` CLI archives for `darwin`, `linux` and `windows` on `amd64` and `arm64` (`.github/workflows/release-studio.yml`). |
| R2 | Studio updates itself in place from the release it was installed from ([Studio release runbook](./STUDIO_RELEASE.md)). |
| R3 | The same release publishes the CLI to npm and to the `esengine/reasonix` Homebrew tap. A stable version such as `2.21.0` moves npm `latest`; a candidate skips Homebrew. |
| R4 | 1.x no longer publishes the CLI to npm, Homebrew or the CLI update pointers. |
| R5 | The next stable `studio-v2.*` release MUST NOT be tagged until every row of [Terminal takeover](#terminal-takeover) is done, because R3 makes it the release that moves `npm i -g reasonix` and `brew install` from 1.x to 2.x. |

## Terminal takeover

The 2.x terminal UI (`reasonix tui`) replaces the 1.x chat screen for people who work in a terminal.

| ID | Item | Status |
| --- | --- | --- |
| T1 | Terminal UI on the in-process kernel: streaming, approvals, questions, queue and steer, `!` shell commands, completion, task list | Done |
| T2 | Look and keys of the 1.x chat screen: banner, framed composer, mode tag, footer telemetry, per-request usage, approval and question panels, i18n | Done |
| T3 | Full screen by default with scrollbar, wheel, drag-select copy, `/mouse`; `--inline` for the terminal's own scrollback | Done |
| T4 | `-r` session picker and `/resume`; system notifications; Ctrl+B for long shell output | Done |
| T5 | Ctrl+V pastes a screenshot (Alt+V on Windows) | Done |
| T6 | Bare `reasonix`, `reasonix -c` and `reasonix -r` start the terminal UI, as they do in 1.x | Not started |
| T7 | Windows checks by hand: IME candidate window position, Alt+V image paste | Not started |
| T8 | Local install test of the npm package and the Homebrew cask before the switch release | Not started |
| T9 | `README.md` and `README.zh-CN.md` name npm and Homebrew as the 2.x CLI channels in the switch release | Not started |

## Other workstreams

| ID | Item | Line | Status |
| --- | --- | --- | --- |
| W1 | 2.x reads 1.x schema 2 session logs (1.x 1.38.2 and later); see [Sessions](./MIGRATING.md#sessions) | 2.x | Not started |
| W2 | 1.x stops listing the `.wire.jsonl`, `.adjudication.jsonl` and `.execution.jsonl` files 2.x writes as conversations | 1.x | Not started |
| W3 | 1.x to 2.x porting list: 1.x fixes, tests and behavior, each marked ported, reimplemented or not carried over | Both | Not started |
| W4 | 2.x contribution and architecture review rules in `CONTRIBUTING.md` | 2.x | Partly done: branch table, dependency direction and cache-first review gate exist |
| W5 | Version note in the 1.x `README.md` on `main-v2` | 1.x | Not started |

## Open decisions

These are decided by @esengine and recorded here when made.

| ID | Decision | State |
| --- | --- | --- |
| D1 | How long 1.x is maintained, and what it receives until then | Open. The announcement says the period will be published separately. |
| D2 | What ends the 2.x pre-release status of Studio | Open |
| D3 | Release cadence of 2.x | Open |
