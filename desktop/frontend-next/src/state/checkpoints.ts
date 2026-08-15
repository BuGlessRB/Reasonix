import type { Checkpoint } from "../port/port";
import type { Item } from "./session";

// The kernel opens one checkpoint per user turn, before the turn's first write,
// so checkpoints and user cards line up in order. Nothing on the wire carries a
// turn number, though — the client mints its own user item — so the pairing is
// positional, and only confirmed when the prompt matches.
//
// A rewind is destructive and cannot itself be undone. So a row that does not
// match gets no entry point at all: showing one against the wrong turn would
// throw away work the user never asked to lose.
export function pairCheckpoints(items: Item[], checkpoints: Checkpoint[]): Map<string, Checkpoint> {
  const pairs = new Map<string, Checkpoint>();
  const users = items.filter((i): i is Extract<Item, { t: "user" }> => i.t === "user");
  let at = 0;
  for (const user of users) {
    // A queued steer is not a turn of its own yet, so it consumes no checkpoint.
    if (user.pending) continue;
    const cp = checkpoints[at];
    if (!cp) break;
    at++;
    if (cp.prompt.trim() === user.text.trim()) {
      pairs.set(user.id, cp);
    }
  }
  return pairs;
}
