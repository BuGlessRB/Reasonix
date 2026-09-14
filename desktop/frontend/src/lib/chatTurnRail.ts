import type { TranscriptOutlineEntry } from "./transcriptProtocol";

/**
 * Which mounted node answers for one outline entry, or undefined while that
 * turn is still unloaded.
 *
 * A message ID is the identity that survives optimistic submission and
 * settlement, so it is tried first; the record ID is the stable fallback that
 * also covers a question that has not been committed yet. Shared by the rail
 * and by the jump transaction so both agree on when a target has really
 * mounted.
 */
export function loadedTurnKey(entry: TranscriptOutlineEntry, mounted: ReadonlySet<string>): string | undefined {
  if (entry.messageId && mounted.has(`m:${entry.messageId}`)) return `m:${entry.messageId}`;
  if (mounted.has(entry.id)) return entry.id;
  return undefined;
}

/** Ordered, de-duplicated outline entries. */
export function alignOutlineEntries(entries: readonly TranscriptOutlineEntry[]): TranscriptOutlineEntry[] {
  const seen = new Set<string>();
  const aligned: TranscriptOutlineEntry[] = [];
  for (const entry of entries) {
    if (seen.has(entry.id)) continue;
    seen.add(entry.id);
    aligned.push(entry);
  }
  aligned.sort((left, right) => left.order - right.order);
  return aligned;
}
