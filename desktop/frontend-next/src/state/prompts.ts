// prompts.ts — the questions a run is stopped on, as the transcript holds them.
// An approval and an ask are the same shape here: the kernel is blocked until
// this window answers, and it re-emits the request to whoever attaches so a
// reloaded page can. What that costs the reducer is identity — the same prompt
// arrives more than once, and it must not become two answerable cards — and
// survival, because a rebuild re-reads the record and a prompt is not in it.
import type { Item, SessionState } from "./session_types";

// The id the kernel correlates an answer by, which is what makes two frames one
// prompt. Local item ids cannot: a replay mints a new one every time.
export function promptKey(it: Item): string | undefined {
  return it.t === "ask" ? it.ask.id : it.t === "approval" ? it.a.id : undefined;
}

// Sealed = answered here already. Open = the kernel is still holding the run on
// it, which is the only kind a rebuild has to carry and a replay may reopen.
export const promptSealed = (it: Item) =>
  (it.t === "ask" && it.answered !== undefined) || (it.t === "approval" && it.verdict !== undefined);

export const promptOpen = (it: Item) => promptKey(it) !== undefined && !promptSealed(it);

// The kernel replays a pending prompt to every client that attaches, and the
// frame log replays the original alongside it, so one question arrives more than
// once. Two cards are two answers it will not both accept — same id, same card,
// and the local id is kept so an answer already in flight still lands on it.
export function prompted(s: SessionState, doing: string, next: Item): SessionState {
  const key = promptKey(next);
  const at = s.items.findIndex((it) => it.t === next.t && promptKey(it) === key);
  if (at < 0) return { ...s, doing, items: [...s.items, next] };
  // A frame replayed from the log for a question already answered is history,
  // not a new wait: reopening it would ask again for a decision already made.
  if (promptSealed(s.items[at])) return s;
  const items = s.items.slice();
  items[at] = { ...next, id: s.items[at].id };
  return { ...s, doing, items };
}
