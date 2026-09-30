# Context retrieval bench

Two questions about long-horizon memory, and the harness that makes their
answers trustworthy.

    -mode=preflight     build all 18 tasks under every arm and check the contract
    -mode=snippet       does a rank-1 hit hand over the whole answer?
    -mode=adversarial   send a model after the answer through every host surface
    -mode=run-search    Experiment R: is searchable canonical a real capability?
    -mode=run-boundary  Experiment I, paired: the 6 efficiency tasks across their boundary
    -mode=run-index     Experiment I, full: the 6 efficiency tasks x 4 budgets
    -mode=calibrate     find each index task's PlantAfterGen for its cue tier

`-dry` drives any run mode with a scripted provider, for nothing. Real runs need
`DEEPSEEK_API_KEY`.

## Status

**Searchable canonical recall: proven.** Six of six recovered with search on,
three of six with it off, and the successes without it came from enumerating
addresses by hand at roughly nine times the page-in cost (139 vs 1292
tokens/task). Measured after three separate leaks were closed, so the numbers
are from a clean environment.

**Fold index marginal utility: unresolved.** Both the positive and the negative
estimates are invalidated. The first Stage 2 ran in a contaminated environment.
The clean paired batch found a consistent effect (six of six tasks searched less
and paged in less with the cue visible), and the same-batch dose-response did
not reproduce it (`boundary-aligned 0/4`, recall-token delta reversing sign).
The most likely reason is that two of the six index tasks carry heavy-tail
stopping behaviour that swamps the effect being measured.

Existing evidence is not sufficient to tune the shipped 1% budget in either
direction. Reopening this wants a new corpus, not more samples: that substrate
now exists, two qualifying tasks per cue tier — see "Index corpus v2" below.

**Things that turned out not to be problems**, each after being measured rather
than argued about:

| Suspected | Measured |
| --- | --- |
| Model writes poor queries (2.17 searches/task) | 22/23 found the target on query 1; the extra searches are one round's parallel fan-out |
| Model reaches for the workspace before memory (7/11) | Linear event order misread; by model round it is MemoryFirst 4/6 |
| Snippets too short for multi-value answers | 240 runes covers 12/12 tasks at 100%; records are 30-179 runes |
| A general stopping failure | 37/44 runs issued no search after the answer was in hand; all 7 that did belong to 2 tasks |

## Measurement distinctions this bench had to learn

Each of these reversed a conclusion once. They are enforced in the schema now,
not left to whoever reads the numbers next.

- **TargetHit is not EvidenceSufficient.** A rank-1 hit can return a window too
  short to answer with. `FirstHitCoverage` records how much of the scored answer
  the first hit actually handed over.
- **Linear event order is not model-round order.** Two calls in one round are a
  parallel fan-out, not a preference. Routing compares rounds.
- **An isolated workdir is not an isolated host.** The sandbox mounts the host
  read-only by design. Three leaks were found and closed: the corpus source, the
  fixture transcript, and its event log sidecar.
- **A count is not its evidence.** Query text, snippet contents and trajectories
  are all persisted now, because three analyses in a row needed a paid re-run to
  ask a question of data already collected.

## How a run stays honest

The corpus holds templates, never answers: every scored literal is a `{{var}}`
instantiated per run, so grepping this directory reveals the question and not
the answer. `TestNoAnswerLiteralExistsInTheRepository` asserts it with
`git grep`. Preflight rebuilds every arm and checks the answer is unreadable in
the real `provider.Request`, the probe query ranks the target within five, and
an index task's cue sits at exactly the scales its tier names. The fixture is
removed from disk once the agent holds it in memory, and the directory is
scanned for answer literals before the first provider request and again after
the turn. Any answer reaching the model through a tool that is not `recall`
marks the run contaminated and takes it out of every statistic.

## Index corpus v2

The substrate is being filled with two tasks per cue tier. These six are the
current candidates, each authored to the bar below and screened three times per
arm: `i09-lease-epoch` and `i10-shard-seal` (quarter), `i07-stream-latch` and
`i08-quota-refill` (half), `i13-replica-tag` and `i14-tombstone-ttl` (default).

Clean means a `stopping_class` that is neither `PostSufficientRetrieval` nor
`PostSufficientRunaway`; three `-mode=run-index` passes (32 cells each) measured:

| task | tier | cue present | cue absent | both arms |
| --- | --- | ---: | ---: | --- |
| `i09-lease-epoch` | quarter | 2/3 | 0/3 | no |
| `i10-shard-seal` | quarter | 0/3 | 2/3 | no |
| `i07-stream-latch` | half | 2/3 | 0/3 | no |
| `i08-quota-refill` | half | 3/3 | 1/3 | no |
| `i11-quorum-floor` | half | 1/3 | 1/3 | no |
| `i12-flush-margin` | half | 1/3 | 1/3 | no |
| `i13-replica-tag` | default | 2/3 | 1/3 | no |
| `i14-tombstone-ttl` | default | 3/3 | 2/3 | no |

No candidate is clean on both arms yet, so this corpus is not qualified:
screening continues and a candidate that does not hold on both arms is replaced
rather than promoted. The classification is also a rate rather than a property at
this sample - which is the variance the freeze note said was unknown - so a
screening batch is three passes per cell and a replacement is measured the same
way. The full per-cell table (eight tasks x four budgets x three passes) is in
the pull request.

`i01`-`i06` are the frozen study's six tasks, kept as `roleStopping`. `i03`/`i04`
are the heavy-tail pair the freeze note names; one screening pass over the other
four showed post-sufficient behaviour in `i01`, `i02` and `i05` (one boundary arm
or both), and `i06` came back clean on both arms. A single pass does not certify
a task - that is what the rates above exist for - so `i06` stays with the frozen
set rather than being drafted into the substrate. All six are excluded from
`run-index` and `run-boundary` batches and stay measurable one at a time with
`-task <id>`. A task added to the corpus declares a role, so it cannot arrive
without one.

A task qualifies only if it is clean on both of its boundary arms - the
cue-present run and the cue-absent run - in every run of a screening batch. Clean
means a `stopping_class` other than `PostSufficientRetrieval` or
`PostSufficientRunaway`: `SnippetStop`, `SnippetThenRead` and `DefensiveRead`
all qualify, and a defensive read is deliberately not counted as waste. The
class comes from the three counts in that report — `searches_after_sufficient`,
`reads_after_sufficient` and `escapes_after_sufficient` — together with how
sufficiency was reached and how many rounds followed it. The remaining
requirements still apply — successful tool call, answer in the output rather
than the cue, rank 1, `FirstHitCoverage` complete, one or two simple values,
nothing in the workspace.

`-mode=calibrate` is not a screening tool: it builds and reads back the
fixtures, and reads nothing else, so its cue-visibility profile is the same for
every task.

Then repeats rather than levels: cue-present against cue-absent on each task's
own boundary, three runs per cell. What is unknown is the variance, and four
budgets sampled once each cannot show it.
