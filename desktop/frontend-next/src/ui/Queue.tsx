import { useCallback, useEffect, useRef, useState } from "react";
import type { Queue as QueueSnapshot, QueueItem } from "../port/port";
import { t } from "../i18n";

interface Props {
  queue: QueueSnapshot | null;
  onRead: (id: string) => Promise<string>;
  onEdit: (id: string, text: string) => void;
  onMove: (id: string, to: number) => void;
  onCancel: (id: string) => void;
  onRetry: (id: string) => void;
  onRefresh: (id: string) => void;
  onPause: (paused: boolean) => void;
}

// What each state means for the row's own affordances. A consumed entry is
// history the kernel has not swept yet: it is past taking back.
const settled = (s: QueueItem["state"]) => s === "steer_consumed" || s === "running";

// Each arm calls t() with its own literal: the catalogue is built by reading
// these call sites, and a table of strings looked up later is invisible to it —
// which is how a row would render Chinese inside an English window.
function label(state: QueueItem["state"]): string {
  switch (state) {
    case "steer_accepted":
      return t("下个工具边界送入");
    case "steer_consumed":
      return t("已送入");
    case "running":
      return t("进行中");
    case "blocked":
      return t("受阻");
    case "uncertain":
      return t("状态不明");
    default:
      return t("等待中");
  }
}

export function Queue({ queue, onRead, onEdit, onMove, onCancel, onRetry, onRefresh, onPause }: Props) {
  const [editing, setEditing] = useState("");
  const [draft, setDraft] = useState("");
  const box = useRef<HTMLTextAreaElement>(null);

  // The body arrives after the click, so focus waits for the field to exist.
  useEffect(() => {
    if (editing) box.current?.focus();
  }, [editing]);

  const open = useCallback(
    async (id: string) => {
      // Opening on the preview would put a cut-off line in the box and save it
      // back as the whole instruction.
      const body = await onRead(id).catch(() => "");
      setDraft(body);
      setEditing(id);
    },
    [onRead],
  );

  const commit = useCallback(() => {
    const text = draft.trim();
    if (editing && text) onEdit(editing, text);
    setEditing("");
  }, [draft, editing, onEdit]);

  if (!queue || (queue.items.length === 0 && !queue.paused)) return null;
  const items = queue.items;
  const full = queue.capacity.maxItems > 0 && queue.capacity.items >= queue.capacity.maxItems;

  return (
    <div className="queue" role="group" aria-label={t("待发队列")}>
      <div className="qhead">
        <span className="qn">{t("待发 {n} 条", { n: items.length })}</span>
        {queue.paused && <span className="qflag" data-hold="">{t("已暂停")}</span>}
        {queue.readonly && <span className="qflag" data-ro="">{t("只读")}</span>}
        {/* The kernel pauses itself when it recovers entries: something was
            said that no one has since re-confirmed. */}
        {!!queue.recoveredCount && <span className="qflag" data-warn="">{t("恢复了 {n} 条", { n: queue.recoveredCount })}</span>}
        {full && <span className="qflag" data-warn="">{t("已满")}</span>}
        <button className="qhold" onClick={() => onPause(!queue.paused)} disabled={queue.readonly}>
          {queue.paused ? t("继续派发") : t("暂停派发")}
        </button>
      </div>

      {items.map((it, i) => (
        <div key={it.id} className="qrow" data-state={it.state} data-intent={it.intent}>
          <span className="qmark" aria-hidden="true">
            {it.intent === "steer" ? "↪" : "→"}
          </span>
          {editing === it.id ? (
            <textarea
              ref={box}
              className="qedit"
              value={draft}
              rows={Math.min(6, draft.split("\n").length)}
              onChange={(e) => setDraft(e.target.value)}
              onBlur={commit}
              onKeyDown={(e) => {
                if (e.key === "Escape") setEditing("");
                // Enter commits; the line is one instruction, and a queue row
                // is not where a paragraph gets composed.
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  commit();
                }
              }}
            />
          ) : (
            <span className="qtext" title={it.preview}>
              {it.preview}
            </span>
          )}
          <span className="qstate">{label(it.state)}</span>
          {it.blockReason && <span className="qwhy">{it.blockReason}</span>}
          {!settled(it.state) && !queue.readonly && editing !== it.id && (
            <span className="qacts">
              <button onClick={() => onMove(it.id, i - 1)} disabled={i === 0} title={t("上移")}>
                ↑
              </button>
              <button onClick={() => onMove(it.id, i + 1)} disabled={i === items.length - 1} title={t("下移")}>
                ↓
              </button>
              <button onClick={() => void open(it.id)} title={t("编辑")}>
                {t("改")}
              </button>
              {it.state === "blocked" && (
                <button onClick={() => onRetry(it.id)} title={t("重试")}>
                  {t("重试")}
                </button>
              )}
              {/* Only an entry that quoted files has anything to re-freeze. */}
              {!!it.refs?.length && (
                <button onClick={() => onRefresh(it.id)} title={t("重新冻结引用的文件")}>
                  {t("刷新")}
                </button>
              )}
              <button className="qdel" onClick={() => onCancel(it.id)} title={t("撤回")}>
                ×
              </button>
            </span>
          )}
        </div>
      ))}
    </div>
  );
}
