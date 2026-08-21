# Reasonix project memory

This file is loaded into every session's system prompt (the cache-stable prefix),
so keep it concise and durable — it is the project's standing instructions to the
agent. It is the Reasonix analog of Claude Code's CLAUDE.md.

## Conventions

- Go kernel under `internal/`; each package owns one concern. A package's long
  explanation belongs in its `doc.go`, not spread across implementation files.
- One transport-agnostic `control.Controller` sits behind every frontend (chat
  TUI, HTTP/SSE serve, Wails desktop). Add behavior to the controller, not a
  frontend, so all three inherit it.
- Layering (enforced): utility packages import nothing under `reasonix/`; only
  the frontends `cli`, `serve`, `acp`, `bot`, `botruntime`, `boot` and the hosts
  `cmd/`, `desktop/` may import `control`; nothing below a frontend may import
  one. The declared sets live in `tools/repolint/layers.go`.
- Subagent delegation keeps five concepts apart: a profile says how a worker
  thinks, `TaskSpec` what this call wants, `CapabilityGrant` what it may touch,
  `ContextRequest` what it starts from, `SchedulerPolicy` when it runs. Put a
  field in whichever member decides its value — profiles carry ceilings, never
  per-call values. `internal/agent/profile_boundary_test.go` enforces it.
- Cache-first: the system-prompt prefix (base prompt + tools + memory) must stay
  byte-stable across turns so DeepSeek's automatic prefix cache stays warm. Never
  mutate it mid-session — ride the turn tail instead (see `control.Compose`).
- Performance features land with an effect test at their final boundary
  (`internal/boot/effect_test.go` pattern): assert what actually reaches the
  provider request, frontend sink, or trajectory through the real `boot.Build`
  assembly. Component correctness is not system effectiveness.
- A mutex- or atomic-guarded struct is ratcheted on its **scalar** field count
  (`struct-state`), not its total: independent flags multiply into states no
  type records as legal. Fixing a boundary case by adding one more `bool` is
  the move this blocks — group by lifetime into a named sub-state instead
  (`agent.perTurnState` is the pattern), which costs one field and removes the
  whole product.
- Judgements read structure, never wording: shell ASTs (`shellparse`), types,
  contracts, tool schemas. Phrase tables and message sniffing have been retired
  in batches (task-policy prose, the planner's approval phrases, the executor
  handoff tables) because each one misfires on real input, and a misfiring
  judgement inside a gate is worse than no gate. Security allow-lists are the
  exception and stay. Something that can only be built by matching words does
  not get built — say what is missing instead of guessing at it.
- The other half of that rule: an error a caller must tell apart **carries an
  identity** — a sentinel, a typed code — never a sentence. Only the producer
  knows a deadline expired rather than a socket closed, and a message is where
  that knowledge goes to die: the reader is left matching words, or with
  nothing. Across HTTP the identity is a dotted code through `refuse`;
  `refusal-path` fails a new `http.Error` in `internal/serve`, and the parity
  test fails a code the frontend cannot say. Inside Go it is `errors.Is` on a
  sentinel the wrapping preserves; `error-text` fails a new match against an
  error message, following it one hop into a local because storing the text
  first is what the direct form turns into.

## Comments

Default is none — the code is the truth. Write one only when the **why** is
non-obvious: a hidden constraint, a workaround anchored to something verifiable,
an invariant the type system cannot express, or an external-protocol quirk.

- Declaration doc: ≤5 lines. Package comment: ≤8 lines, or ≤40 in a `doc.go`.
- Every other comment: ≤3 lines. Struct-field and trailing `//`: 1 line.
- Never: restatements of the code, phase/stage narrative, incident or
  conversation history, section banners, commented-out code, `@param` lists.
- `TODO(#nnn):` and `HACK(#nnn):` need the issue anchor. `FIXME` is banned.
- One responsibility per file; 800 lines is the ceiling. Test files are exempt:
  their length tracks how many cases they cover, not how many concerns they
  carry, so splitting one only scatters a subject's table across files.

`go run ./tools/repolint` enforces all of it against a ratchet baseline: recorded
debt is tolerated, anything new fails CI. Never widen the baseline to land a
change — fix the code. `-update` lowers budgets freely and refuses to raise one
without `-allow-widen`, so carrying debt through a rename or an extraction is
asked for in the command and justified in the PR. A clean run also reports
budget the tree stopped using: it is only reclaimed by an `-update`, and until
then it is room a file can grow back into.

## Memory

- Standing instructions are hierarchical: committed/shared `REASONIX.md`,
  `AGENTS.md`, and `CLAUDE.md`; personal `*.local.md` variants; matching files in
  ancestor directories; and user-global files under the memory state root
  (`REASONIX_STATE_HOME`, otherwise `REASONIX_HOME`, otherwise `~/.reasonix` on
  macOS/Linux or `%APPDATA%\reasonix` on Windows). All distinct supported files
  in a directory load; `AGENTS.md` is not merely a fallback.
- `@path` on its own line imports another file's contents.
- `#<note>` in chat quick-adds an always-on instruction. The `remember` tool
  instead saves a fallible background fact (frontmatter file + `MEMORY.md`
  index). Fact `type` classifies content; independent `scope` controls whether it
  is project-only (the default) or explicitly global. The index loads into the
  stable prefix on the next session; global user/feedback bodies also load as
  lower-priority compatibility guidance. The current turn receives a tail note.

## Notes

One tool call costs one model round trip, so calls that do not depend on each
other belong in the same round: several reads, edits to different files, a check
after the edit that enables it. Several edits to the *same* file are one
`multi_edit` — it applies them against each other in memory and rewrites the
file only if all of them land, so a failure midway leaves nothing half-edited.

## Pre-push CI simulation

Run these **before every commit** to catch the fastest CI failures locally:

```bash
gofmt -w .                          # catches gofmt (saves ~13s CI)
go vet ./...                        # catches vet warnings (saves ~52s CI/lint)
make lint                           # golangci-lint at CI's pin + repolint
go test ./internal/tool/builtin/ ./internal/boot/  # catches tool/boot test breaks
```

`make lint` runs both gates CI runs, at the version in `.golangci-version`;
`make lint-install` installs it. Do not skip it: a `modernize` finding never
shows up in `go vet`, and the CI round trip that catches it instead costs ten
minutes.

A full repolint run reports every file over budget, including debt your change
never touched. To see only what your own edits owe:

```bash
make check
```

Repo-wide ceilings still report under `-only`, because a file can push the tree
past one without exceeding its own budget.

Run it through `make`, not as `go run ./tools/repolint -only ...`. The host
recognizes a `make` target as a check whose result it can read, while an
arbitrary `go run ./tools/...` is a program it has to assume writes — so only
the `make` form can be cited as evidence that this gate passed.

## Import cycle rule

Before importing a new internal package from a non-test file, verify the target package's **test files** aren't already importing back to you:

```
# BAD: agent(_test.go) → tool/builtin(sessions.go) → agent  → setup failed
```

Use `go test ./path/to/target/` to detect cycles **before** pushing. A `[setup failed]` message means a cycle exists.

## PR hygiene

- **One force-push per round of review feedback.** Multiple force-pushes destroy review history and confuse reviewers.
- **Keep the PR diff minimal.** Only the files relevant to the PR's purpose — no stray changes from other branches.
- **Amend, don't add commits, for review feedback** — keeps the commit history clean.

## PR metadata gates

Two CI guards read the PR body. The scripts are the source of truth and both
run locally: `scripts/check-cache-impact.sh`, `scripts/check-docs-impact.sh`.
Separators must be an ASCII `-` or `:` — an em dash fails the docs guard.

Cache-sensitive diffs (`internal/tool/`, `internal/provider/`,
`internal/boot/`, `internal/agent/agent.go`, and the rest of the list in the
script) require:

```
Cache-impact: <none|low|medium|high> - <reason>
Cache-guard: <focused guard test/command or existing guard rationale>
```

`none` is a legitimate impact when the provider-visible prefix stays
byte-identical; only an empty value, `todo`, or `tbd` is rejected. If the diff
also touches `internal/config/`, `internal/memory/`, `internal/outputstyle/`,
`internal/skill/`, or `internal/boot/`, add `System-prompt-review: <note>` —
that field additionally rejects `none` and `n/a`, so it must name a reviewer.

User-visible diffs (`cmd/reasonix/`, `desktop/`, `npm/`, and most of
`internal/`; tests and lockfiles are exempt) require one of these, chosen by
whether the same PR edited `docs/*.md`:

```
Documentation-impact: updated - <what changed>            # docs/*.md edited
Documentation-impact: none - <why the docs stay correct>  # not edited
```

## Reasonix host checks

The paths below decide what the agent is allowed to do — a wrong change here
is not a bug in a feature, it is a hole in a boundary. Declaring them makes
every change under them demand `review` plus `security_review` before a turn
can finish. Sensitivity is declared, never inferred: the host does not read it
out of how a path is spelled, which cannot tell `internal/auth` from
`session_write_authority.go`, or a trace file from a data race.

- sensitive: internal/permission/**
- sensitive: internal/sandbox/**
- sensitive: internal/shellsafe/**
- sensitive: internal/shellparse/**
- sensitive: internal/control/approval.go
- sensitive: internal/installsource/**
- sensitive: internal/plugin/**
- sensitive: internal/pluginpkg/**
- sensitive: internal/netclient/**
