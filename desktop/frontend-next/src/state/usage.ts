import type { Metrics } from "./session_types";
import type { Usage } from "../port/wire";

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

// foldUsage folds one billed round into the session's running metrics. It lives
// here, not in the reducer, because every line of it is about money, tokens and
// what the round was sampled against.
export function foldUsage(m: Metrics, u: Usage): Metrics {
  const src = u.source || "executor";
  const spent = quoteAmount(u.costQuote) ?? u.cost ?? 0;
  const d = u.cacheDiagnostics;
  return {
    ...m,
    hit: m.hit + u.cacheHitTokens,
    miss: m.miss + u.cacheMissTokens,
    out: m.out + u.completionTokens,
    bySource: { ...m.bySource, [src]: (m.bySource[src] ?? 0) + spent },
    cost: m.cost + spent,
    // The kernel sends both, and says the code is preferred: only it can tell
    // CN¥ from ¥ in an English window. The symbol stays as the fallback.
    currency: u.currencyCode || u.costQuote?.original.currency || u.currency || m.currency,
    // Diagnostics describe the round that just billed, so they replace rather
    // than accumulate. A round that carried none leaves the last answer
    // standing instead of blanking the block mid-turn.
    prefixHash: d?.prefixHash || m.prefixHash,
    prefixChanged: d ? d.prefixChanged : m.prefixChanged,
    prefixReasons: d?.prefixChangeReasons ?? (d ? [] : m.prefixReasons),
    bodyChanged: d ? d.bodyChanged : m.bodyChanged,
    carriedMessages: d?.carriedMessages ?? m.carriedMessages,
    toolSchema: d?.toolSchemaTokens ?? m.toolSchema,
    estimated: u.costQuote?.estimated ?? u.estimated ?? m.estimated,
    alt: altAmount(u.costQuote) ?? m.alt,
    turn: m.turn + spent,
    rounds: sampleRound(m.rounds, u.totalTokens),
  };
}
