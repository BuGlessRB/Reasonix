import { useRef, useState, type ReactNode } from "react";
import { useStartsOpen } from "../../state/foldpref";
import { StudioIcon } from "../StudioIcon";
import { t } from "../../i18n";
import { count, decimals } from "../../i18n/format";
import { Sym } from "../Sym";
import type { Item } from "../../state/session";
import { LazyMarkdown } from "../LazyMarkdown";
import { Boundary } from "../Boundary";
import { CopyButton } from "../CopyButton";
import { useRevealed } from "../reveal";
import { useDismiss } from "../dismiss";

// Folded, the only thing left of a thought is how much of the turn it was. The
// spec puts both halves there — how long, and how much — because either alone
// hides whether a slow turn was spent thinking or waiting.
function thoughtLabel(item: Extract<Item, { t: "say" }>) {
  // Graphemes, not code units: an emoji in the reasoning is one character to
  // the reader and two to the string.
  const chars = t("{n} 字", { n: count([...(item.reasoning ?? "")].length) });
  return item.thoughtMs
    ? t("思考 {secs} 秒 · {chars}", { secs: decimals(item.thoughtMs / 1000, 1), chars })
    : t("思考 {chars}", { chars });
}

// A ref is "provider/model"; the model is what identifies it to a reader, and
// the provider stays in the title for when two of them carry the same name.
const modelName = (ref: string) => ref.slice(ref.lastIndexOf("/") + 1);

/** A quote on its way to the composer. n makes the same text twice two
 *  requests; turn is the kernel's name for the reply it came from, absent when
 *  the transcript was rebuilt without checkpoints. */
export interface Quote {
  text: string;
  turn?: number;
  n: number;
}

/** What a finished reply can be acted on with. Absent members are capabilities
 *  this transcript does not have — a rebuilt one cannot re-run a turn — and the
 *  bar draws only what it can actually do. */
export interface ReplyActions {
  onQuote: (text: string, id: string) => void;
  onRegenerate?: () => void;
  model?: string;
  onConfigureModel?: () => void;
  onRunDetail?: () => void;
}

// Quoting a whole answer to ask about one sentence of it is not quoting. What
// the reader has highlighted inside this card is what they mean; the selection
// has to be checked against the card because the browser keeps the last one
// anywhere on the page.
function selectedIn(card: Element | null): string {
  const sel = card && window.getSelection();
  if (!sel || sel.isCollapsed || sel.rangeCount === 0) return "";
  const range = sel.getRangeAt(0);
  if (!card.contains(range.commonAncestorContainer)) return "";
  return sel.toString().trim();
}

function download(text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: "text/markdown;charset=utf-8" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = "reasonix-reply.md";
  link.click();
  URL.revokeObjectURL(url);
}

export function SayCard({ item, afterAnswer, reply }: { item: Extract<Item, { t: "say" }>; afterAnswer?: ReactNode; reply?: ReplyActions }) {
  const start = useStartsOpen("thinking", !item.done && !item.text);
  const [touched, setOpen] = useState<boolean | null>(null);
  const open = touched ?? start;
  const [menu, setMenu] = useState<"" | "retry" | "more">("");
  // The menu draws above this row, outside its box, so leaving the row is the
  // way to reach it — not the way to dismiss it.
  const acts = useRef<HTMLDivElement>(null);
  useDismiss(!!menu, acts, () => setMenu(""));
  // Thinking is the longest-running stream of the turn — 10s of it before the
  // first answer token, measured — so it gets the same paced reveal the answer
  // does rather than tracking the wire's bursts.
  const thought = useRevealed(item.reasoning ?? "", !item.done);
  return (
    <div className="call" data-k="say">
      <div className="g">
        <Sym glyph="◦" />
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className="nm">reasonix</span>
          {/* Which model wrote this one. A turn can carry two, and without it
              both read as whatever the composer happens to name now. */}
          {item.model && <span className="src" title={item.model}>{modelName(item.model)}</span>}
        </div>
        <div className="out">
          {item.reasoning?.trim() && (
            <details className="think" open={open} onToggle={(e) => e.currentTarget.open !== open && setOpen(e.currentTarget.open)}>
              <summary>
                <span className="fold">{item.done ? thoughtLabel(item) : t("思考中…")}</span>
              </summary>
              <div className="tk">
                {thought}
                {!item.done && <span className="caret" />}
              </div>
            </details>
          )}
          {item.text && (
            <div className="txt">
              <Boundary fallback={<div className="md">{item.text}</div>}>
                <LazyMarkdown text={item.text} streaming={!item.done} />
              </Boundary>
            </div>
          )}
          {afterAnswer}
          {/* Only once the answer is whole: copying half a stream hands over
              something that was never said. */}
          {item.done && item.text.trim() && (
            <div className="acts" ref={acts}>
              <CopyButton text={item.text} iconOnly />
              {reply && (
                <button type="button" data-action="reply.quote" title={t("引用到输入框")} aria-label={t("引用到输入框")} onClick={(e) => reply.onQuote(selectedIn(e.currentTarget.closest(".call")) || item.text, item.id)}>
                  <StudioIcon name="quote" />
                </button>
              )}
              {reply?.onRegenerate && (
                <span className="acts-menu">
                  <button type="button" data-action="reply.retry" title={t("重新生成")} aria-label={t("重新生成")} aria-expanded={menu === "retry"} onClick={() => setMenu((m) => (m === "retry" ? "" : "retry"))}>
                    <StudioIcon name="refresh" />
                  </button>
                  {menu === "retry" && (
                    <div className="acts-pop" role="menu">
                      <div className="acts-pop-head">{t("重新生成")}<small>{t("当前回复会留在运行历史里")}</small></div>
                      <button type="button" role="menuitem" data-action="reply.retry-now" onClick={() => { setMenu(""); reply.onRegenerate?.(); }}>
                        <StudioIcon name="refresh" /><span>{t("按当前配置重试")}</span>{reply.model && <small>{reply.model}</small>}
                      </button>
                      {reply.onConfigureModel && (
                        <button type="button" role="menuitem" data-action="reply.configure" onClick={() => { setMenu(""); reply.onConfigureModel?.(); }}>
                          <StudioIcon name="sliders" /><span>{t("先调整模型与强度")}</span>
                        </button>
                      )}
                    </div>
                  )}
                </span>
              )}
              {reply && (
                <span className="acts-menu">
                  <button type="button" data-action="reply.more" title={t("更多")} aria-label={t("更多")} aria-expanded={menu === "more"} onClick={() => setMenu((m) => (m === "more" ? "" : "more"))}>
                    <StudioIcon name="more" />
                  </button>
                  {menu === "more" && (
                    <div className="acts-pop" role="menu">
                      <div className="acts-pop-head">{t("这条回复")}</div>
                      <button type="button" role="menuitem" data-action="reply.download" onClick={() => { setMenu(""); download(item.text); }}>
                        <StudioIcon name="download" /><span>{t("下载回复")}</span><small>Markdown</small>
                      </button>
                      {reply.onRunDetail && (
                        <>
                          <div className="acts-pop-head">{t("本轮执行")}</div>
                          <button type="button" role="menuitem" data-action="reply.run-detail" onClick={() => { setMenu(""); reply.onRunDetail?.(); }}>
                            <StudioIcon name="gauge" /><span>{t("运行分析")}</span>
                          </button>
                        </>
                      )}
                    </div>
                  )}
                </span>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
