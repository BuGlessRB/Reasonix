import type { TranscriptOutlineEntry } from "./transcriptProtocol";

/** The identities a loaded user turn exposes to the rail. */
export interface LoadedTurnNode {
  /** The item's own id, which is also its DOM anchor key. */
  readonly id: string;
  readonly messageId?: string;
}

/**
 * The mounted node that answers for one outline entry, or undefined while that
 * turn is still unloaded.
 *
 * Matches on identity, never on the shape of the node key: a question submitted
 * in this app session keeps its optimistic `u<seq>` id after the authoritative
 * message arrives and only gains a `messageId`, so key-shaped comparisons would
 * miss exactly the turns the reader just wrote. A message ID is the identity
 * that survives settlement, so it wins; the record ID is the stable fallback
 * that also covers a question that has not been committed yet.
 *
 * Shared by the rail and by the jump transaction so both agree on when a target
 * has really mounted.
 */
export function findLoadedTurn(
  order: readonly string[],
  read: (key: string) => LoadedTurnNode | undefined,
  entry: TranscriptOutlineEntry,
): string | undefined {
  let fallback: string | undefined;
  for (const key of order) {
    const node = read(key);
    if (node === undefined) continue;
    if (entry.messageId !== undefined && node.messageId === entry.messageId) return key;
    if (fallback === undefined && node.id === entry.id) fallback = key;
  }
  return fallback;
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
