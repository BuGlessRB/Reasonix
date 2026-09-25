---
owner: @esengine
backup: @SivanCola
status: active
reviewed: 2026-09-25
---

# Moving from Reasonix 1.x to 2.x

## Purpose

This guide is for people who run Reasonix 1.x and want to try or move to 2.x. It lists what the two lines share, what does not carry over, and the steps to run both on one machine.

- Why there are two release lines: the [version roadmap announcement](https://github.com/esengine/DeepSeek-Reasonix/discussions/10748).
- What 2.x still has to deliver: the [roadmap](./ROADMAP.md).

## Release lines

| | Reasonix 1.x | Reasonix 2.x |
| --- | --- | --- |
| Branch | `main-v2` | `studio` (default) |
| Status | Maintenance / stable | Active development, pre-release |
| Desktop app | 1.x desktop, from the [download page](https://reasonix.io/?download=desktop#start) | Reasonix Studio, from the [`studio-v2.*` releases](https://github.com/esengine/DeepSeek-Reasonix/releases?q=studio-v&expanded=true) |
| CLI | `npm i -g reasonix`, or `brew install esengine/reasonix/reasonix` | Archives attached to each `studio-v2.*` release |
| Issue label | `v2` | `v3` |

## What the two lines share

Both lines read and write the same Reasonix home: `~/.reasonix` on macOS and Linux, `%APPDATA%\reasonix` on Windows. See [Configuration Paths](./CONFIG_PATHS.md).

| Data | Path | Shared |
| --- | --- | --- |
| Global config | `<home>/config.toml` | Yes. 2.x keeps sections it does not use, such as `[bot]`, unchanged when it rewrites the file. |
| Provider keys | `<home>/.env` | Yes |
| Slash commands, skills, hooks | `<home>/commands/`, `<home>/skills/`, `<home>/settings.json` | Yes |
| Memory | `<home>/memory/`, `<home>/projects/` | Yes |
| Project instructions | `REASONIX.md`, `AGENTS.md`, `CLAUDE.md` in the project | Yes |
| Sessions | `<home>/projects/<project>/sessions/`; 1.x 1.38.8 or later also `sessions-v4/` | Partly. See [Sessions](#sessions). |

## Sessions

Where 1.x keeps a conversation depends on the 1.x version that last saved it.

| Saved by | Kept in | Opens in 2.x |
| --- | --- | --- |
| 1.x before 1.38.2 | `sessions/`, event log schema 1 | Yes |
| 1.x 1.38.2 to 1.38.7 | `sessions/`, event log schema 2 | Yes |
| 1.x 1.38.8 or later | `sessions-v4/<id>/` | Yes. Opening one imports it to `sessions/v4-<id>.jsonl`. |
| 2.x | `sessions/`, event log schema 1 | Yes |

| ID | Rule |
| --- | --- |
| S1 | 2.x never writes a file of a session 1.x saved. Opening one and leaving it changes nothing. |
| S2 | Continuing a `sessions/` 1.x session in 2.x moves it to a new 2.x session with the same title before the first turn runs, and says so. 1.x keeps the original. |
| S3 | 1.x 1.38.8 or later copies an older session into `sessions-v4` when it opens it. The copy under `sessions/` stays as it was, and 2.x still opens it. |
| S4 | A `sessions-v4` conversation is imported to `sessions/v4-<id>.jsonl` the first time 2.x opens it. If 1.x continues it later, 2.x refreshes the import on the next open while the import has no 2.x turns; after it has, the two copies go their own ways. |
| S6 | `reasonix session list --json` lists 2.x transcripts only; a `sessions-v4` conversation appears there once it has been imported. |
| S5 | 1.x lists the `.wire.jsonl`, `.adjudication.jsonl` and `.execution.jsonl` files 2.x writes as extra conversations. Ignore them in 1.x; they belong to the 2.x session with the same name. |

## Commands

| 1.x | 2.x |
| --- | --- |
| `reasonix`, `reasonix -c`, `reasonix -r` | The same, in a terminal. `reasonix tui` is another name for it. |
| `--permission-mode workspace-write` | Auto |
| `--permission-mode danger-full-access` | Yolo |
| `--permission-mode read-only` | Ask. 2.x has no read-only mode, so this is the most careful one it has. |
| `--yolo`, `--permission-mode yolo` or `bypassPermissions` | Yolo. In 1.x these meant `workspace-write`; in 2.x they skip ordinary approval prompts. |
| `reasonix run`, `serve`, `web`, `acp`, `mcp`, `setup`, `doctor` | Same names |
| `reasonix bot` | Not in 2.x. The `[bot]` config section stays for 1.x. |
| VS Code extension | Starts the `reasonix` found on `PATH`, so it runs whichever line's CLI comes first there. |

## Steps: run 2.x beside 1.x

1. Download Reasonix Studio for your platform from the latest [`studio-v2.*` release](https://github.com/esengine/DeepSeek-Reasonix/releases?q=studio-v&expanded=true) and install it. Studio updates itself after that.
2. For the 2.x terminal UI, download the `reasonix` archive for your platform from the same release and unpack it into a directory of your choice.
3. Start the 2.x terminal UI from the unpacked directory:

   ```sh
   ./reasonix tui
   ```

4. Report 2.x problems with **Version line: 2.x** in the issue form, and the version from Studio's settings or `reasonix --version`.

## Steps: go back to 1.x

1. Keep using the 1.x desktop app or `reasonix` from npm or Homebrew. Config, keys, skills and memory are the same files.
2. Sessions 2.x saved open in 1.x. 1.x 1.38.8 or later copies them into `sessions-v4` first (rule S3).
