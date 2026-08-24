import type { Tool } from "../port/wire";
import { t } from "../i18n";
import { seconds, tokens } from "../i18n/format";

// What a step cost, in the two units the kernel can state honestly per call:
// wall-clock, and what the call left in the prompt. The spec pairs them in that
// order wherever both are known ("1m04s · 9.5k"), so every card reads the same
// way and the slot never means one thing here and another there.
// 墙钟跨三个数量级 —— 一次读文件 0.2s，一条 bash 一分钟 —— 线性映射会把快的
// 那一批全挤成一个点。对数摊开它们，条只说长短，是什么类别由轨道那条线说。
function tookWidth(ms: number) {
  return 3 + Math.log10(1 + (ms / 1000) * 9) * 22;
}

export function Cost({ tools }: { tools: Tool[] }) {
  const ms = tools.reduce((n, t) => n + (t.durationMs ?? 0), 0);
  const left = tools.reduce((n, t) => n + (t.contextTokens ?? 0), 0);
  if (!ms && !left) return null;
  return (
    <span className="cost">
      {ms > 0 && <i className="took" style={{ width: `${tookWidth(ms).toFixed(1)}px` }} aria-hidden="true" />}
      {ms > 0 && <span>{seconds(ms)}</span>}
      {left > 0 && <span title={t("这一步留在上下文里的估算 token")}>{tokens(left)}</span>}
    </span>
  );
}
