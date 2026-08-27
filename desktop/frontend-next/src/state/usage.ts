// What a billed round reports, read out of the quote rather than guessed.
// These live apart from the reducer because they answer about money and token
// counts, not about how a session's items fold together.

// The quote is the authoritative amount: it carries currency and an estimated
// flag, while usage.cost is only a legacy alias.
export function quoteAmount(q?: { selected?: { amount: string }; original: { amount: string } }): number | undefined {
  const raw = q?.selected?.amount ?? q?.original.amount;
  if (raw === undefined) return undefined;
  const n = Number(raw);
  return Number.isFinite(n) ? n : undefined;
}

// The second currency, when the host quoted one. Only a *converted* quote has
// two sides; a single-currency quote reports the same money twice and is not
// two rows.
// ROUNDS bounds the per-round series: a session that runs for hours must not
// grow an array in the reducer, and a trend older than the last few dozen
// rounds is not what the sparkline is being read for.
const ROUNDS = 48;

export function sampleRound(prev: number[], total: number): number[] {
  if (!total) return prev;
  const next = prev.length >= ROUNDS ? prev.slice(1) : prev.slice();
  next.push(total);
  return next;
}

export function altAmount(q?: {
  selected?: { amount: string; currency: string };
  original: { amount: string; currency: string };
}): { amount: number; currency: string } | null {
  if (!q?.selected || q.selected.currency === q.original.currency) return null;
  const amount = Number(q.original.amount);
  return Number.isFinite(amount) ? { amount, currency: q.original.currency } : null;
}
