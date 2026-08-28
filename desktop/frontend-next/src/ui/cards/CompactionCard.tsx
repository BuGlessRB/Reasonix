import type { Compaction } from "../../port/wire";
import { Sym } from "../Sym";
import { t } from "../../i18n";
import { tokens } from "../../i18n/format";
import { tx } from "../../i18n/rich";

const TRIGGER: Record<string, string> = {
  auto: "上下文到阈值，自动触发",
  manual: "你手动触发",
};

// A digest reads as complete whatever it dropped, so the count of the fold's
// changes it actually carried is the one thing this card can say that the
// summary text cannot. It is stated whether or not anything is missing: a
// number that only appears on failure is a number nobody learns to read.
function Coverage({ c }: { c: Compaction }) {
  const required = c.coverageRequired ?? 0;
  if (required === 0) return null;
  const missing = c.coverageMissing ?? 0;
  const kept = required - missing;
  return (
    <div className="comp-l">
      <div className="row">
        <span className="k">✓</span>
        <span>
          {missing === 0 ? (
            <>{tx("这段做过的 {n} 处改动，简报都写到了", { n: <b>{required}</b> })}</>
          ) : (
            <>{tx("{kept}/{required} 处改动写进了简报", { kept: <b>{kept}</b>, required })}</>
          )}
          {c.coverageRepaired && t("（补写过一次）")}
          {!c.coverageRepaired && c.coverageBackstopped && t("（缺的由主机补上）")}
        </span>
      </div>
      {missing > 0 && (
        <div className="row">
          <span className="x">×</span>
          <span>
            {tx("还有 {n} 处只剩下索引地址，要用原文得 recall 取回", { n: <b>{missing}</b> })}
          </span>
        </div>
      )}
    </div>
  );
}

// The tail of what has been written, kept to one line: a digest is thousands
// of tokens of headings and lists, and growing the card by all of them would
// move everything below it for the length of the fold.
function lastLine(text: string): string {
  const line = text.trimEnd().split("\n").at(-1)?.trim() ?? "";
  return line.length > 96 ? "…" + line.slice(-96) : line;
}

export function CompactionCard({ c, done }: { c: Compaction; done: boolean }) {
  const before = c.sourceTokens ?? 0;
  const after = c.projectionTokens ?? 0;
  const shrank = before > 0 && after > 0 && after < before;
  return (
    <div className="call">
      <div className="g">
        <Sym glyph="⊘" />
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className={done ? "nm" : "nm shim"}>{t(done ? "压缩完成" : "正在压缩…")}</span>
          <span className="tag">compaction</span>
          {c.trigger && <span className="arg">{TRIGGER[c.trigger] ?? c.trigger}</span>}
        </div>
        {/* While it runs, the digest's own last line is the progress bar: a
            fold can take a minute, and a placeholder that says nothing for a
            minute cannot be told apart from one that has hung. */}
        {!done && (
          <div className="out">
            <div className="comp">
              {/* Before the first word of the digest arrives there is still
                  something true to say: how much is being folded. Without it
                  the card sits blank for the length of a model call. */}
              {(c.messages || before > 0) && (
                <div className="comp-n">
                  {c.messages ? (
                    <>{tx("正在把 {n} 条消息折成简报", { n: <b>{c.messages}</b> })}</>
                  ) : (
                    t("正在折成简报")
                  )}
                  {before > 0 && (
                    <>
                      {" · "}
                      <b>{tokens(before)}</b>
                    </>
                  )}
                </div>
              )}
              {c.summary && <div className="comp-n">{lastLine(c.summary)}</div>}
            </div>
          </div>
        )}
        {done && (
          <div className="out">
            <div className="comp">
              {/* The filled part is what the fold gave back, not what it kept:
                  the bar's fill is the ok colour, and more of it has to mean
                  more room. */}
              {shrank && (
                <div className="comp-bar" title={`${before} → ${after} tokens`}>
                  <i style={{ width: `${Math.max(2, Math.round((1 - after / before) * 100))}%` }} />
                </div>
              )}
              <div className="comp-n">
                {c.messages ? (
                  <>{tx("折叠了 {n} 条消息", { n: <b>{c.messages}</b> })}</>
                ) : (
                  t("这一趟没折叠掉什么")
                )}
                {shrank && (
                  <>
                    {" · "}
                    <b>{tokens(before)}</b> → <b>{tokens(after)}</b>
                  </>
                )}
              </div>
              <Coverage c={c} />
              {c.summary && (
                <details>
                  <summary>
                    <span className="fold">{t("看它接着往下用的简报")}</span>
                  </summary>
                  <div className="txt">{c.summary}</div>
                </details>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
