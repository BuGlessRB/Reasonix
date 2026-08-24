# fleet-fanout

Four tasks, two pairs. Each pair answers the same question over the same tree and
differs in one item only: the `map-per-subject` half gives the middle item
`for_each`, so it runs one child per subject; the `single-reviewer` half gives it
`depends_on`, so one child handles them all. The second is what the same call had
to look like before `for_each` existed, which makes it the honest comparison —
`for_each` is a capability, so there is no "the same thing with the payload
removed" arm the way the dependency edge has one.

- `map-per-subject` / `single-reviewer` — twelve near-identical handlers, one
  missing a validation call. Per-subject work is one short read.
- `heavy-map-per-subject` / `heavy-single-reviewer` — ten modules, each hiding
  its effective timeout behind a three-step derivation, one above the cap.
  Grepping the constants answers nothing, so per-subject work is real reasoning.
  This pair was written to be the case mapping should win.

Both pairs solved on both arms in every run, and mapping cost 85% (p=0.002) and
98% (p=0.029) more tokens for it. `for_each` therefore ships off; see
`internal/boot/delegation_tools.go`. Enabling it there is what these suites are
for — as is deciding to delete it.

The prompts fix the graph shape in both arms. That is an experimental control,
not a claim about how often a model reaches for either shape.

```sh
go run ./cmd/e2ebench -suite benchmarks/fleet-fanout -bin <reasonix> -json run.json
```
