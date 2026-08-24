# fleet-fanout

Two tasks that answer the same question over the same twelve files and differ in
one item only: `map-per-subject` gives the middle item `for_each`, so it runs one
child per handler; `single-reviewer` gives it `depends_on`, so one child handles
all twelve. The second is what the same call had to look like before `for_each`
existed, which makes it the honest comparison — `for_each` is a capability, so
there is no "same thing with the payload removed" arm the way the dependency
edge has one.

The prompts fix the graph shape in both arms. That is an experimental control,
not a claim about how often a model reaches for either shape.

```sh
go run ./cmd/e2ebench -suite benchmarks/fleet-fanout -bin <reasonix> -json run.json
```
