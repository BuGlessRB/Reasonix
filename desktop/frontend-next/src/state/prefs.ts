// Per-machine display choices. They live in localStorage rather than in the
// kernel's settings because they answer "what does this screen show me",
// which is not a fact about the session and does not travel with it.

// Off unless this machine asked for it: the card reports what went unverified,
// which is worth reading and easy to tire of seeing after every turn.
const RECEIPT_KEY = "rx-turn-receipt";

export function showsReceipt(): boolean {
  try {
    return localStorage.getItem(RECEIPT_KEY) === "on";
  } catch {
    return false;
  }
}
