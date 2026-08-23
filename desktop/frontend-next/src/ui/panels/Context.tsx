import { useEffect, useRef, useState, type RefObject } from "react";
import { t } from "../../i18n";
import { useTicker } from "../num";
import type { ContextBreakdown } from "../../port/port";
import { pct as percent, tokens } from "../../i18n/format";
import { pinToViewport } from "../place";

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


// Gap between the bar and the bubble. It lives here rather than in CSS because
// a fixed bubble is placed by measurement, so no rule owns the offset any more.
const GAP = 9;

// The metrics rail scrolls, and a scroller clips: an absolutely positioned
// bubble lost its left edge to the middle pane as soon as the rail was dragged
// narrower than the bubble. Fixed lifts it out of the scroller; the size stays
// CSS's, read back off the element so no measurement is copied into JS.
function place(anchor: RefObject<HTMLElement | null>) {
  return (el: HTMLDivElement | null) => {
    if (!el || !anchor.current) return;
    const to = anchor.current.getBoundingClientRect();
    const box = el.getBoundingClientRect();
    const above = to.top - box.height - GAP;
    pinToViewport(el, to.right - box.width, above >= 6 ? above : to.bottom + GAP);
  };
}

/** Context is the gauge plus what fills it. The gauge alone says a session is
 *  at 70% without saying whether that is a tool catalogue, a memory file, or
 *  one enormous output — and those are fixed in completely different ways. The
 *  breakdown stays folded because it is a diagnosis, not a running number. */
export function Context({ ctx }: { ctx: ContextBreakdown | null }) {
  // Every hook runs before the first return: ctx arrives one render after the
  // rail mounts, and a guard above them made that render ask for hooks the
  // previous one never did.
  const used = useTicker(ctx?.used || 0);
  const [open, setOpen] = useState(false);
  const bar = useRef<HTMLDivElement>(null);

  // A bubble placed once against the viewport goes stale the moment anything
  // moves it, and there is nothing useful to show mid-scroll — so it closes.
  useEffect(() => {
    if (!open) return;
    const close = () => setOpen(false);
    addEventListener("scroll", close, true);
    addEventListener("resize", close);
    return () => {
      removeEventListener("scroll", close, true);
      removeEventListener("resize", close);
    };
  }, [open]);

  if (!ctx) return null;
  // A window nobody declared has no denominator to draw against — but the
  // number still matters, and so does what else a zero window means: it is
  // what turns automatic compaction off. Vanishing said neither.
  if (!ctx.window) {
    return (
      <div className="block" data-b="ctx">
        <div className="lbl">
          {t("上下文")}
          <span className="n">{tokens(Math.round(used))}</span>
        </div>
        <p className="ctxnote">
          {t("没人说过这个来源的窗口有多大，所以画不出用了多少 —— 也不会自动压缩。去「连接」里给它填一个上下文窗口。")}
        </p>
      </div>
    );
  }
  const pct = Math.min((used / ctx.window) * 100, 100);
  const parts = PARTS.map(([k, label, why]) => ({ k, label, why, n: ctx[k] || 0 })).filter((p) => p.n > 0);
  const sum = parts.reduce((a, p) => a + p.n, 0) || 1;

  return (
    <div className="block" data-b="ctx">
      <div className="lbl">
        {t("上下文")}
        <span className="n">
          {tokens(Math.round(used))} / {tokens(ctx.window)}
        </span>
      </div>
      {/* Hover, not a permanent list: five more rows of numbers in a rail this
          narrow costs more than it tells anyone who is not debugging. */}
      <div
        className="ctxbar"
        ref={bar}
        tabIndex={0}
        role="group"
        aria-label={t("上下文构成")}
        onMouseEnter={() => setOpen(true)}
        onMouseLeave={() => setOpen(false)}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
      >
        {parts.map((p) => (
          <i key={p.k} data-p={p.k} style={{ width: `${(p.n / sum) * pct}%` }} />
        ))}
      </div>
      {open && (
        <div className="ctxpop" role="tooltip" ref={place(bar)}>
          <div className="hd">
            <span>{t("上下文构成")}</span>
            <span className="n">{percent(pct / 100)}</span>
          </div>
          {parts.map((p) => (
            <div className="row" key={p.k} title={p.why}>
              <i data-p={p.k} />
              <span className="t">{p.label}</span>
              <span className="v">{tokens(p.n)}</span>
              <span className="p">{percent(p.n / sum)}</span>
            </div>
          ))}
          <p className="foot">{t("估算值，和触发压缩用的是同一把尺子")}</p>
        </div>
      )}
    </div>
  );
}
