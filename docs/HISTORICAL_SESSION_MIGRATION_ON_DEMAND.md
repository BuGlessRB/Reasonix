# On-demand historical session migration

## Scope

Desktop startup no longer converts every legacy transcript or canonical v4
directory. Startup only repairs small, non-historical lifecycle reservations.
The historical catalog reads directory entries, published head indexes, and
durable registry metadata. Legacy rows participate in the normal sidebar and
history search; opening one starts preparation in the conversation navigation
flow and switches to the canonical target only after preparation commits.

The management surface is **Settings → Storage → Historical sessions**. Trash
contains archived/deleted canonical sessions only.

## Ownership and lifecycle

- A source import takes a per-source cross-process lock and, for canonical
  stores, a shared directory ownership lock for the complete copy and commit.
- A source already owned by another CLI or runtime returns a blocked source
  status immediately. It never waits behind the desktop runtime rebuild lock.
- Duplicate requests for one source join the same revisioned preparation task.
  Interactive navigation and a bulk batch hold separate demands, so cancelling
  a batch cannot cancel a session that the user is currently opening.
- Batch import is sequential, cancellable, and resumable. Its selected source
  snapshot is stored in `historical-import-queue.v1.json` with an atomic,
  cross-process-locked update. After restart it is paused until the user
  explicitly continues. Cancellation leaves
  `prepared` and `content_ready` reservations for the next explicit attempt.
- Existing `content_ready` operations are replayed against their durable target;
  they are not converted into a second session.
- Archive and purge state remains authoritative. Deleted or archived sessions
  are not resurrected by a later catalog scan.
- `PrepareSession`, `GetSessionPreparation`, and
  `CancelSessionPreparation` expose `queued`, `preparing`, `ready`, `blocked`,
  `failed`, and `cancelled` scheduling states without adding lifecycle phases.
- Source content checks compare durable bytes rather than timestamps or title
  metadata. A confirmed version can be explicitly imported with
  `PrepareHistoricalSourceVersion`; its `:review:<fingerprint>` mapping is a
  separate branch while the original mapping remains stable.

## Compatibility

Source files remain unchanged. Existing source mappings, recovery entries,
unknown fields, presentations, and retained artifacts are preserved. The
path-only legacy route remains an alias for the selected DAG head; valid head
indexes expose alternate heads as separate on-demand sources. A missing or
stale index degrades to one source row and never causes event-log replay in a
listing RPC.

The queue sidecar is scheduling intent only. The workspace lifecycle registry
remains authoritative for target Session IDs and commit state. Older builds
ignore the sidecar and optional RPC fields; committed sessions and retained
sources remain readable after rollback. Preparation metadata is never added to
model prompts or transcript messages.

## Verification

The implementation has deterministic coverage for startup non-migration,
cross-process cold-export contention, duplicate and cancelled imports,
prepared/content-ready resume, revisioned duplicate preparation, paused queue
restart, archive/purge fencing, and browser interactions for listing, retry,
open-after-commit, and batch controls.
