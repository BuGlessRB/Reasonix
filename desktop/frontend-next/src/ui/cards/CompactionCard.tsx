import type { Compaction } from "../../port/wire";

const TRIGGER: Record<string, string> = {
  auto: "上下文到阈值，自动触发",
  manual: "你手动触发",
};

// CompactionStarted carries only the trigger; everything else arrives on Done,
// and an aborted pass leaves the summary empty. There is no before/after count
// on the wire, so nothing here draws a ratio.
export function CompactionCard({ c, done }: { c: Compaction; done: boolean }) {
  return (
    <div className="call">
      <div className="g">
        <span className="sym">⊘</span>
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className={done ? "nm" : "nm shim"}>{done ? "压缩完成" : "正在压缩…"}</span>
          <span className="tag">compaction</span>
          {c.trigger && <span className="arg">{TRIGGER[c.trigger] ?? c.trigger}</span>}
        </div>
        {done && (
          <div className="out">
            <div className="comp">
              <div className="comp-n">
                {c.messages ? (
                  <>
                    折叠了 <b>{c.messages}</b> 条消息
                  </>
                ) : (
                  "这一趟没折叠掉什么"
                )}
                {c.archive && <>，原件留在 {c.archive}</>}
              </div>
              {c.summary && (
                <details>
                  <summary>
                    <span className="fold">看它接着往下用的简报</span>
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
