# progress_corpus

Extracts execution-progress metrics from a Reasonix trajectory export.

```bash
python3 tools/trajectory/progress_corpus.py session.json
python3 tools/trajectory/progress_corpus.py session.json --json --require-semantic
python3 tools/trajectory/progress_corpus.py session.json --transitions out.jsonl
```

Standard library only. Tests: `python3 -m unittest discover -s tools/trajectory -t tools/trajectory`.

## Three statements this tool is built on

**Total task tokens measure resource consumption; progress gaps measure
liveness. They are not interchangeable.** A task that spends 120M tokens
advancing steadily and one that spends 120M without advancing after the first
hour have the same total and nothing else in common.

**Proxy samples are suitable for historical incident analysis but must not be
mixed with semantic samples when calibrating stopping policy.** The two count
different things, and a quantile over both is a quantile over neither.

**Reported thresholds are descriptive, not prescriptive.** Nothing here
recommends a budget. A number that fits one incident is a property of that
incident until a corpus says otherwise.

## Two sources of progress

`progress_source` and `progress_fidelity` are part of every sample, not a note
in the output.

| source | fidelity | what advances the counter |
| --- | --- | --- |
| `todo_progress` | `semantic` | the canonical task list moved: a step completed and execution went on |
| `complete_step` | `proxy` | a sign-off call succeeded |

The proxy is what an export predating the `todo_progress` frame can offer. It
**overcounts**: a sign-off landing on a step already complete is a renewal, and
renewals advance nothing.

That direction is what makes one proxy number still usable. Removing false
advances can only merge adjacent gaps, never split them, so

```
proxy max_tokens_between_progress   is a lower bound on the semantic one
proxy max_wall_between_progress     is a lower bound on the semantic one
```

Everything else changes shape when the false events go: `p50`, `p90`,
`max/p50`, the event count, and the transition mix all need the canonical
session to recompute. The table above is why `--require-semantic` exists —
a corpus meant to calibrate policy must refuse a silent downgrade.

## The incident this was built from

A 15.6-hour session, read in proxy mode:

```
total_tokens                393.95M
progress_events             25          (proxy — an upper bound)
p50_tokens_between_progress 0.22M
max_tokens_between_progress 348.81M     (88.5% of the total, 12.32h)
max_to_p50_ratio            1581x
content_to_progress_ratio   43.6
```

The two bounds that survive the proxy caveat: at least 348.81M tokens and at
least 739 minutes passed between two host-observed advances. That is the claim
liveness work rests on, and it is stated in resources rather than rounds.

## Known fragility

Semantic transitions are read back from the rendered trajectory line
(`todo advance · content 4 · plan 1 · progress 2`), because that is what an
export carries. A renderer change breaks the parse — deliberately loudly: a
line that starts with `todo ` and does not parse raises rather than reading as
"no semantic progress", which would quietly turn every later sample into a
proxy. If the export ever carries the frame's fields structurally, read those
instead.
