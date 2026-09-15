import { useEffect, useRef, useState } from "react";
import type { Checkpoint, RewindPlan, RewindResult, RewindScope } from "../../port/port";
import type { Item } from "../../state/session";
import { RewindControl } from "./RewindControl";
import { reason } from "../../i18n/kernel";
import { t } from "../../i18n";

export function UserCard({
  item,
  cp,
  onResend,
  onCancelQueued,
  onPrepareRewind,
  onCommitRewind,
  onUndoRewind,
}: {
  item: Extract<Item, { t: "user" }>;
  cp?: Checkpoint;
  onResend?: (turn: number, text: string) => Promise<void>;
  onCancelQueued?: (rowId: string, itemId: string) => void;
  onPrepareRewind?: (turn: number, scope: RewindScope) => Promise<RewindPlan>;
  onCommitRewind?: (planId: string) => Promise<RewindResult>;
  onUndoRewind?: (transactionId: string) => Promise<void>;
}) {
  // A queued line is the only thing on screen that has not happened yet, which
  // makes it the only thing that can still be taken back. The id is the
  // kernel's, and it goes away the moment the turn reads the line.
  const queued = item.pending ? item.itemId : undefined;
  // A rewind needs a turn the kernel claimed, and a queued line has not
  // happened yet — there is nothing behind it to take back.
  const editable = !!onResend && !!cp && !item.pending;
  const [draft, setDraft] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [failed, setFailed] = useState("");
  const box = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    const el = box.current;
    if (draft === null || !el) return;
    el.focus();
    el.setSelectionRange(el.value.length, el.value.length);
  }, [draft === null]);

  const resend = () => {
    const text = (draft ?? "").trim();
    if (!editable || !text || sending) return;
    setSending(true);
    setFailed("");
    onResend!(cp!.turn, text)
      .then(() => setDraft(null))
      .catch((e) => setFailed(reason(e)))
      .finally(() => setSending(false));
  };

  return (
    <div className="call" data-k="me" data-pending={item.pending ? "" : undefined}>
      <div className="g">
        <span className="sym">{t("你")}</span>
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl user-hl">
          {item.pending && (
            <span className="pend">
              {item.queued === "followup" ? t("排队中 · 本轮结束后发送") : t("排队中 · 下一个工具边界送达")}
            </span>
          )}
          {queued && onCancelQueued && (
            <button className="pcancel" data-action="queue.cancel" data-target={item.id} onClick={() => onCancelQueued(item.id, queued)}>
              {t("撤回")}
            </button>
          )}
          {/* The entry point lives on the turn it returns to, so there is no
              list to read and no turn number to match up by eye. */}
          {editable && draft === null && (
            <button
              className="reask-open"
              data-action="turn.edit"
              data-target={item.id}
              title={t("改写这条消息并重新发送")}
              onClick={() => setDraft(item.text)}
            >
              {t("✎ 改写")}
            </button>
          )}
          {cp && onPrepareRewind && onCommitRewind && onUndoRewind && (
            <RewindControl cp={cp} onPrepare={onPrepareRewind} onCommit={onCommitRewind} onUndo={onUndoRewind} />
          )}
        </div>
        <div className="out">
          {draft === null ? (
            <div className="txt">{item.text}</div>
          ) : (
            <div className="reask">
              <textarea
                ref={box}
                data-action-change="turn.edit"
                data-action-keydown="turn.resend"
                data-target={item.id}
                value={draft}
                rows={Math.min(12, draft.split("\n").length + 1)}
                readOnly={sending}
                aria-label={t("改写这条消息")}
                onChange={(ev) => setDraft(ev.target.value)}
                onKeyDown={(ev) => {
                  if (ev.key === "Escape") {
                    ev.preventDefault();
                    ev.stopPropagation();
                    setDraft(null);
                  } else if (ev.key === "Enter" && !ev.shiftKey && !ev.nativeEvent.isComposing) {
                    ev.preventDefault();
                    resend();
                  }
                }}
              />
              {failed && <div className="txt bad">{failed}</div>}
              <div className="reask-ft">
                <button
                  className="btn"
                  data-action="turn.resend"
                  data-target={item.id}
                  disabled={sending || !draft.trim()}
                  onClick={resend}
                >
                  {sending ? t("正在重发…") : t("改完重发")}
                </button>
                <button className="dismiss" data-action="turn.edit" data-value="cancel" onClick={() => setDraft(null)}>
                  {t("取消")}
                </button>
                <span className="hint">{t("这一轮之后的记录会被丢弃")}</span>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
