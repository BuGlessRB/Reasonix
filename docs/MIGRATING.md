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
| Sessions | `<home>/projects/<project>/sessions/` | Partly. See [Sessions](#sessions). |

## Sessions

1.x 1.38.2 and later save sessions in an event log format (schema 2) that 2.x cannot read yet. Opening such a session in 2.x fails with `session event log for … uses schema 2; this build supports up to 1`.

| Session last saved by | Opens in 1.x | Opens in 2.x |
| --- | --- | --- |
| 2.x | Yes | Yes |
| 1.x before 1.38.2 | Yes | Yes |
| 1.x 1.38.2 or later, without `REASONIX_SESSION_LOG=v1` | Yes | No |

| ID | Rule |
| --- | --- |
| S1 | A 2.x session that 1.x 1.38.2 or later continues is upgraded to schema 2 in place and stops opening in 2.x. |
| S2 | Neither line converts a schema 2 session back. |
| S3 | Starting 1.x with `REASONIX_SESSION_LOG=v1` keeps sessions it has not upgraded yet on schema 1. Sessions already upgraded stay schema 2. |
| S4 | A person who runs both lines on one machine SHOULD set `REASONIX_SESSION_LOG=v1` for 1.x until 2.x reads schema 2. |
| S5 | 1.x lists the `.wire.jsonl`, `.adjudication.jsonl` and `.execution.jsonl` files 2.x writes as extra conversations. Ignore them in 1.x; they belong to the 2.x session with the same name. |

## Commands

| 1.x | 2.x |
| --- | --- |
| `reasonix`, `reasonix -c`, `reasonix -r` | `reasonix tui`, `reasonix tui -c`, `reasonix tui -r`. Bare `reasonix` prints usage in 2.x. |
| `reasonix run`, `serve`, `web`, `acp`, `mcp`, `setup`, `doctor` | Same names |
| `reasonix bot` | Not in 2.x. The `[bot]` config section stays for 1.x. |
| VS Code extension | Starts the `reasonix` found on `PATH`, so it runs whichever line's CLI comes first there. |

## Steps: run 2.x beside 1.x

1. Download Reasonix Studio for your platform from the latest [`studio-v2.*` release](https://github.com/esengine/DeepSeek-Reasonix/releases?q=studio-v&expanded=true) and install it. Studio updates itself after that.
2. For the 2.x terminal UI, download the `reasonix` archive for your platform from the same release and unpack it into a directory of your choice.
3. Keep 1.x from upgrading sessions 2.x still has to open. Add this to your shell profile:

   ```sh
   export REASONIX_SESSION_LOG=v1
   ```

   On Windows:

   ```powershell
   setx REASONIX_SESSION_LOG v1
   ```

4. Start the 2.x terminal UI from the unpacked directory:

   ```sh
   ./reasonix tui
   ```

5. Report 2.x problems with **Version line: 2.x** in the issue form, and the version from Studio's settings or `reasonix --version`.

## Steps: go back to 1.x

1. Keep using the 1.x desktop app or `reasonix` from npm or Homebrew. Config, keys, skills and memory are the same files.
2. Sessions 2.x saved open in 1.x. Continuing one in 1.x 1.38.2 or later upgrades it to schema 2 (rule S1).
