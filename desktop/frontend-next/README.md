---
owner: @esengine
backup: @SivanCola
status: active
reviewed: 2026-09-22
---

# Reasonix Studio frontend

## Tests

`pnpm test` runs one worker per thread and reuses it across files (`isolate:
false`). Re-importing React, the port and `app.css` once per file was most of
the run; sharing the registry cut it roughly in half.

What that costs: module-level state outlives a test file. A test that needs a
clean module makes one — a fresh instance, an explicit reset — rather than
assuming the file boundary gave it one. `pnpm vitest run --sequence.shuffle`
is how that assumption gets caught.

`css: true` is load-bearing: the guards under `src/styles` read `app.css`
through `?raw`, and a stubbed stylesheet hands them an empty string, which
passes on nothing.
