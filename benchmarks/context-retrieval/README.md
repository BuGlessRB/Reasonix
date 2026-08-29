# Context retrieval bench

Two questions about long-horizon memory, and the harness that makes their
answers trustworthy.

    -mode=preflight     build all 12 tasks under every arm and check the contract
    -mode=snippet       does a rank-1 hit hand over the whole answer?
    -mode=adversarial   send a model after the answer through every host surface
    -mode=run-search    Experiment R: is searchable canonical a real capability?
    -mode=run-boundary  Experiment I, paired: 6 tasks across their own cue boundary
    -mode=run-index     Experiment I, full: 6 tasks x 4 index budgets
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
direction. Reopening this wants a new corpus, not more samples: see
"Index corpus v2" below.

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

## Index corpus v2, if this is reopened

The present index tasks are not a fit substrate: two of six carry stopping
behaviour that dominates the signal. A task should qualify only if a calibration
run shows `PostSufficientSearches == 0`, alongside the existing requirements
(successful tool call, answer in the output rather than the cue, rank 1,
`FirstHitCoverage` complete, one or two simple values, nothing in the
workspace). Keep i03 and i04 — as a stopping corpus, not an efficiency one.

Then repeats rather than levels: cue-present against cue-absent on each task's
own boundary, three runs per cell. What is unknown is the variance, and four
budgets sampled once each cannot show it.
