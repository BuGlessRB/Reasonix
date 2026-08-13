import type { CompletionSummary } from "../../port/wire";
import { Sym } from "../Sym";

type Lvl = "ok" | "warn" | "err" | "";
type Row = { k: string; t: string; why?: string; lvl: Lvl };

const VERDICT: Record<string, Row> = {
  complete: { k: "v", t: "目标达成", lvl: "ok" },
  partial: { k: "v", t: "只做了一部分", why: "剩下的没做完，别当成收工", lvl: "warn" },
  blocked: { k: "v", t: "被卡住了", why: "这一轮没能推进", lvl: "err" },
  continue: { k: "v", t: "还在往下做", lvl: "" },
};

// The kernel's gap kinds, verbatim from emitCompletionSummary.
const GAP: Record<string, string> = {
  suppressed: "有检查项被压掉了",
  stale_check: "有检查跑在改动之前，结论已经过期",
  suppressed_requirement: "有必做项被压掉了",
};

const REVIEW: Record<string, Row> = {
  passed: { k: "r", t: "独立复核过了", lvl: "ok" },
  unavailable: { k: "r", t: "这轮要求独立复核，但没有复核记录", lvl: "warn" },
  warned: { k: "r", t: "复核提了意见", lvl: "warn" },
  failed: { k: "r", t: "复核没通过", lvl: "err" },
};

function rowsOf(c: CompletionSummary): Row[] {
  const rows: Row[] = [VERDICT[c.verdict] ?? { k: "v", t: c.verdict, lvl: "" }];
  if (c.mutations > 0) rows.push({ k: "m", t: `动了 ${c.mutations} 处`, lvl: "" });
  const { checks_passed: ok, checks_failed: bad, checks_suppressed: off } = c;
  if (ok + bad + off > 0) {
    const parts = [`${ok} 过`, bad ? `${bad} 挂` : "", off ? `${off} 压掉` : ""].filter(Boolean);
    rows.push({ k: "c", t: `检查 ${parts.join(" · ")}`, lvl: bad ? "err" : off ? "warn" : "ok" });
  }
  const review = REVIEW[c.review];
  if (review) rows.push(review);
  for (const g of c.gap_kinds ?? []) rows.push({ k: `g${g}`, t: GAP[g] ?? g, lvl: "warn" });
  if (c.constraint_degraded) {
    rows.push({ k: "d", t: "本轮的验证被约束限住了", why: "禁用了测试或只允许指定检查项", lvl: "warn" });
  }
  return rows;
}

export function CompletionCard({ c }: { c: CompletionSummary }) {
  const rows = rowsOf(c);
  return (
    <div className="call">
      <div className="g">
        <Sym glyph="✓" />
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className="nm">收工检查</span>
          <span className="tag">{c.preset}</span>
        </div>
        <div className="out">
          <div className="finds">
            {rows.map((r) => (
              <div className="find" key={r.k} data-lvl={r.lvl || undefined}>
                <span className="t">{r.t}</span>
                {r.why && <span className="why">{r.why}</span>}
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
