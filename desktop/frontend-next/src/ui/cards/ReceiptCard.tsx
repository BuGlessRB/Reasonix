import type { Receipt } from "../../port/wire";
import { t } from "../../i18n";

// The end-of-turn receipt. It reports evidence, never scope: the host knows
// what it could and could not verify, and it knows nothing about how much of
// the ask is left. A card that turns "unproven" into "half-finished" contradicts
// the turn the user just watched, so no verdict noun is rendered at all —
// what is missing is listed, and what carried a clean answer is sourced.
//
// Weights, lightest first: a clean turn settles into one ghost line; a turn's
// own declarations sit under a muted heading with no severity bar, because
// somebody volunteered them; only what the host found missing gets the bar.

// A switch of literal t() calls rather than a lookup table: the catalogue gate
// reads t("…") out of the source, and a table's values are invisible to it —
// which is how the card this replaced shipped an all-Chinese English window.
// An unknown kind prints as itself; dropping one is the failure this card
// exists to prevent. declared_unverified is retired but stored sessions replay
// the words they were written with.
function gapPhrase(kind: string): string {
  switch (kind) {
    case "unbacked_claim": return t("声称过但账本不支持");
    case "unproven_criterion": return t("验收项没有证据");
    case "missing_check": return t("预期的检查从未通过");
    case "failed_verification": return t("验证失败");
    case "stale_verification": return t("验证早于最后一次改动");
    case "inconclusive_verification": return t("跑过，但退出码是命令里另一段的");
    case "unverified_change": return t("改动了但没有任何验证");
    case "unreviewed_change": return t("改动后再没看过");
    case "declared_unverified": return t("自己申报未验证");
    default: return kind;
  }
}

// A receipt long enough to scroll is a receipt nobody reads; the count keeps
// the total honest.
const MAX_GAPS = 5;

// What carried a clean verdict, so "verified" is never an unsourced assertion.
function evidence(r: Receipt): string[] {
  const out: string[] = [];
  if (r.changes?.length) out.push(t("{n} 个文件", { n: r.changes.length }));
  for (const v of r.verifications ?? []) {
    if (v.passed && !v.stale && !v.inconclusive) out.push(v.command);
  }
  return out;
}

// Verified is the settling line: what carried the clean answer, sourced.
function Verified({ r }: { r: Receipt }) {
  const parts = evidence(r);
  return (
    <div className="rc rc-ok">
      <span className="rc-tick">✓</span>
      <span className="rc-t">{t("没有未经验证的部分")}</span>
      {parts.length > 0 && <span className="rc-src">{parts.join(" · ")}</span>}
    </div>
  );
}

export function ReceiptCard({ r }: { r: Receipt }) {
  const gaps = r.gaps ?? [];
  const declared = [...(r.unverified ?? []), ...(r.risks ?? [])];
  const clean = gaps.length === 0;

  if (clean && declared.length === 0) return <Verified r={r} />;

  const shown = gaps.slice(0, MAX_GAPS);
  const rest = gaps.length - shown.length;
  return (
    <div className="rc">
      {clean && <Verified r={r} />}
      {gaps.length > 0 && (
        <section className="rc-group">
          <span className="rc-hd">{t("未验证")}</span>
          {shown.map((g, i) => (
            <div className="rc-row" key={`${g.kind}:${g.detail ?? i}`} style={{ "--i": i } as React.CSSProperties}>
              <span className="rc-t">{gapPhrase(g.kind)}</span>
              {g.detail && (
                <code className="rc-d" title={g.detail}>
                  {g.detail}
                </code>
              )}
            </div>
          ))}
          {rest > 0 && <span className="rc-more">{t("另有 {n} 项", { n: rest })}</span>}
        </section>
      )}
      {declared.length > 0 && (
        <section className="rc-group" data-declared>
          {/* Volunteered, not found — so it carries no severity bar. */}
          <span className="rc-hd">{t("这一轮自己说明的")}</span>
          {declared.map((d, i) => (
            <div className="rc-row" key={d} style={{ "--i": gaps.length + i } as React.CSSProperties}>
              <span className="rc-t">{d}</span>
            </div>
          ))}
        </section>
      )}
    </div>
  );
}
