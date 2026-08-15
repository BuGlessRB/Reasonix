import type { Tool } from "../port/wire";

// What a step cost, in the two units the kernel can state honestly per call:
// wall-clock, and what the call left in the prompt. The spec pairs them in that
// order wherever both are known ("1m04s · 9.5k"), so every card reads the same
// way and the slot never means one thing here and another there.
export function tokenLabel(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${(n / 1000).toFixed(n < 10_000 ? 1 : 0)}k`;
  return `${(n / 1_000_000).toFixed(1)}M`;
}

export const secondsLabel = (ms: number) => `${(ms / 1000).toFixed(1)}s`;

export function Cost({ tools }: { tools: Tool[] }) {
  const ms = tools.reduce((n, t) => n + (t.durationMs ?? 0), 0);
  const tokens = tools.reduce((n, t) => n + (t.contextTokens ?? 0), 0);
  if (!ms && !tokens) return null;
  return (
    <span className="cost">
      {ms > 0 && <span>{secondsLabel(ms)}</span>}
      {tokens > 0 && <span title="这一步留在上下文里的估算 token">{tokenLabel(tokens)}</span>}
    </span>
  );
}
