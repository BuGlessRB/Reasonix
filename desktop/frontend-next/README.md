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
the run; sharing the registry took it from about 19s to about 9s.

What that costs: module-level state outlives a test file. A test that needs a
clean module makes one — a fresh instance, an explicit reset — rather than
assuming the file boundary gave it one. `pnpm vitest run --sequence.shuffle`
is how that assumption gets caught.

`vi.mock` is the one thing that cannot share a registry: a module another file
already imported unmocked stays unmocked, and the mock silently does nothing.

- Those files run in a second, isolated project.
- Which files they are is read out of the sources at config load, never listed.
  A list is what stops matching the moment somebody writes the next one.
- A config load that finds no test files fails rather than quietly covering
  nothing.

`css: true` is load-bearing: the guards under `src/styles` read `app.css`
through `?raw`, and a stubbed stylesheet hands them an empty string, which
passes on nothing.
