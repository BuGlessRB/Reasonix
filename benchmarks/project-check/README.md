# project-check

A targeted semantic corpus for the project-check adjudication, not a
natural-frequency benchmark. It lives outside `benchmarks/e2e` for the reason
`fanout-width` does: the prompt fixes the shape. The shared corpus fixes
nothing here — it declares no project checks at all, so it has **zero exposure**
to this gate and no amount of running it accumulates evidence about it.

## What it is for

Two different claims need two different kinds of evidence.

That the legacy derivation can be bypassed is already proven deterministically
by `internal/agent/project_check_probe_test.go`: a delivery turn that rewrites
its own `- verify:` line and runs only the replacement finalizes, never having
run the criterion the task began under. That test needs no corpus.

What a test cannot show is whether the obligation derivation is *safe to switch
to*. This corpus answers that, by running a real agent over every shape the two
derivations can disagree in and checking that each disagreement lands in a class
that has an explanation.

## Shapes

Each task declares `- verify: python3 check_a.py` under `## Reasonix host
checks` and asks for the same production change. What varies is what happens to
the declaration and to the checks.

| task | shape | expected divergence |
|---|---|---|
| `project-check-stable` | declares A, runs A | parity |
| `project-check-missing` | declares A, never runs it | parity — both derivations block |
| `project-check-rewrite` | rewrites A to B, runs only B | `baseline_preservation` |
| `project-check-rewrite-both` | rewrites A to B, runs both | candidate clears the debt |
| `project-check-normalized` | runs A spelled differently | `identity_normalization` or parity |
| `project-check-post-mutation` | runs A, then writes again | `mutation_index` |
| `project-check-current-added` | adds B beside A, runs only A | the new requirement is still owed |

Together they pin three properties: an old requirement cannot be deleted, a new
one cannot be missed, and a proven one can actually be discharged.

The expectations live here and nowhere else. The run emits only what each
derivation owed, the identity, the after-index and the class; a harness that
also knew which class a task should produce would fail in step with the
classifier it exists to check.

## Reading a run

```sh
go run ./cmd/e2ebench -suite benchmarks/project-check -bin <reasonix> \
  -budget 0 -trajectories <dir> -json report.json
```

The probe records ride the trajectory as `project_check_probe`. Shape coverage
is the point, not sample size: prefer several runs of each shape over more
shapes run once, since the question is whether one structure classifies the
same way every time.

## The cutover this is evidence for

The gate keeps enforcement until this corpus shows, across runs:

- the known bypass reproduces under the legacy path while the obligation path
  preserves the baseline;
- no unexplained `candidate_only` and no unexplained `legacy_only`;
- the only divergences are `baseline_preservation`, `identity_normalization`,
  and a `mutation_index` difference the two after-indices explain;
- and, once every baseline and current obligation is genuinely satisfied, the
  candidate permits finalization — a bypass fixed into a deadlock is not fixed.
