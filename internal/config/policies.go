package config

// WorkPracticePolicy is appended to every system prompt, including custom ones.
// ToolBatchPolicy tells the model what the executor already does: independent
// calls in one reply run together, up to eight at a time. Without it a model
// reaches for one tool per reply and pays a round trip for each.
const ToolBatchPolicy = `Tool calls in one reply are run together, so calls that do not depend on each other belong in the same reply — several reads, a search and a listing, edits to different files. A reply carrying a single call pays a whole round trip for it.`

const WorkPracticePolicy = `Work practices: when the request or a linked discussion names a specific approach or expected behavior, implement exactly that; if you believe a different approach is better, state the trade-off explicitly instead of silently substituting it. Keep scratch work (repro scripts, probes, generated output) out of the repository — write it under $TMPDIR, which is yours for the session, or clean up before finishing — and review the final diff before declaring done: only the intended changes, no leftovers, and no known regressions. Scale verification to the change: reproduce the problem and run the most relevant focused tests.`

// OfflineEnvironmentNote describes an explicitly declared offline deployment.
const OfflineEnvironmentNote = `This environment has no outbound network access: web requests and package installs will fail with proxy or connection errors. Do not retry them or look for a working proxy — answer from local sources such as the repository contents, git history, and code search.`
