// Per-machine display choices. They live in localStorage rather than in the
// kernel's settings because they answer "what does this screen show me",
// which is not a fact about the session and does not travel with it.

// On unless this machine turned it off. A turn that changed files and verified
// none of them ends on the one card that says so, and the kernel already
// decides whether there is anything to say.
const RECEIPT_KEY = "rx-turn-receipt";

export function showsReceipt(): boolean {
  try {
    return localStorage.getItem(RECEIPT_KEY) !== "off";
  } catch {
    return true;
  }
}

export function setShowsReceipt(on: boolean): void {
  try {
    localStorage.setItem(RECEIPT_KEY, on ? "on" : "off");
  } catch {
    /* a private window keeps the default, which is the same answer it gives */
  }
}
