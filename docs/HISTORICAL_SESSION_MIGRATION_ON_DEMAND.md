# On-demand historical session migration

## Scope

Desktop startup no longer converts every legacy transcript or canonical v4
directory. Startup only repairs small, non-historical lifecycle reservations.
The historical catalog reads directory entries and durable registry metadata;
opening a row is the explicit import request.

## Ownership and lifecycle

- A source import takes a per-source cross-process lock and, for canonical
  stores, a shared directory ownership lock for the complete copy and commit.
- A source already owned by another CLI or runtime returns a blocked source
  status immediately. It never waits behind the desktop runtime rebuild lock.
- Duplicate requests for one source join the same in-process call.
- Batch import is sequential, cancellable, and resumable. Cancellation leaves
  `prepared` and `content_ready` reservations for the next explicit attempt.
- Existing `content_ready` operations are replayed against their durable target;
  they are not converted into a second session.
- Archive and purge state remains authoritative. Deleted or archived sessions
  are not resurrected by a later catalog scan.

## Compatibility

Source files remain unchanged. Existing source mappings, recovery entries,
unknown fields, presentations, and retained artifacts are preserved. The
path-only legacy route remains an alias for the selected DAG head; valid head
indexes expose alternate heads as separate on-demand sources without replaying
the event log during listing.

## Verification

The implementation has deterministic coverage for startup non-migration,
cross-process cold-export contention, duplicate and cancelled imports,
prepared/content-ready resume, archive/purge fencing, and browser interactions
for listing, retry, open-after-commit, and batch controls.
