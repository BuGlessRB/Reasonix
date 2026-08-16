import { t } from "../../i18n";
import type { ContextBreakdown } from "../../port/port";

// The order is the order they arrive in a prompt, so the bar reads the way the
// request is built rather than by size — a class that grows is easier to spot
// when its neighbours stay put.
const PARTS: [keyof ContextBreakdown, string, string][] = [
  ["system", t("系统提示"), t("基础指令、记忆、技能清单")],
  ["tools", t("工具定义"), t("发给模型的工具清单")],
  ["user", t("你说的话"), t("这一会话里你输入的部分")],
  ["reply", t("模型回复"), t("模型说过的话")],
  ["output", t("工具输出"), t("命令、读取、检索返回的内容")],
];

const fmt = (n: number) => (n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n));

/** Context is the gauge plus what fills it. The gauge alone says a session is
 *  at 70% without saying whether that is a tool catalogue, a memory file, or
 *  one enormous output — and those are fixed in completely different ways. The
 *  breakdown stays folded because it is a diagnosis, not a running number. */
export function Context({ ctx }: { ctx: ContextBreakdown | null }) {
  if (!ctx || !ctx.window) return null;
  const used = ctx.used || 0;
  const pct = Math.min((used / ctx.window) * 100, 100);
  const parts = PARTS.map(([k, label, why]) => ({ k, label, why, n: ctx[k] || 0 })).filter((p) => p.n > 0);
  const sum = parts.reduce((a, p) => a + p.n, 0) || 1;

  return (
    <div className="block" data-b="ctx">
      <div className="lbl">
        {t("上下文")}
        <span className="n">
          {fmt(used)} / {fmt(ctx.window)}
        </span>
      </div>
      {/* Hover, not a permanent list: five more rows of numbers in a rail this
          narrow costs more than it tells anyone who is not debugging. */}
      <div className="ctxbar" tabIndex={0} role="group" aria-label={t("上下文构成")}>
        {parts.map((p) => (
          <i key={p.k} data-p={p.k} style={{ width: `${(p.n / sum) * pct}%` }} />
        ))}
        <div className="ctxpop" role="tooltip">
          <div className="hd">
            <span>{t("上下文构成")}</span>
            <span className="n">{pct.toFixed(0)}%</span>
          </div>
          {parts.map((p) => (
            <div className="row" key={p.k} title={p.why}>
              <i data-p={p.k} />
              <span className="t">{p.label}</span>
              <span className="v">{fmt(p.n)}</span>
              <span className="p">{Math.round((p.n / sum) * 100)}%</span>
            </div>
          ))}
          <p className="foot">{t("估算值，和触发压缩用的是同一把尺子")}</p>
        </div>
      </div>
    </div>
  );
}
