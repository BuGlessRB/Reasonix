import type { Tool } from "../port/wire";
import { t } from "../i18n";
import { seconds, tokens } from "../i18n/format";

// What a step cost, in the two units the kernel can state honestly per call:
// wall-clock, and what the call left in the prompt. The spec pairs them in that
// order wherever both are known ("1m04s · 9.5k"), so every card reads the same
// way and the slot never means one thing here and another there.
export function Cost({ tools }: { tools: Tool[] }) {
  const ms = tools.reduce((n, t) => n + (t.durationMs ?? 0), 0);
  const left = tools.reduce((n, t) => n + (t.contextTokens ?? 0), 0);
  if (!ms && !left) return null;
  return (
    <span className="cost">
      {ms > 0 && <span>{seconds(ms)}</span>}
      {left > 0 && <span title={t("这一步留在上下文里的估算 token")}>{tokens(left)}</span>}
    </span>
  );
}
