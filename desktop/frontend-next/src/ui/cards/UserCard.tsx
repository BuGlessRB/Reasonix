import type { Checkpoint, RewindPlan, RewindResult, RewindScope } from "../../port/port";
import type { Item } from "../../state/session";
import { RewindControl } from "./RewindControl";
import { t } from "../../i18n";

export function UserCard({
  item,
  cp,
  onPrepareRewind,
  onCommitRewind,
  onUndoRewind,
}: {
  item: Extract<Item, { t: "user" }>;
  cp?: Checkpoint;
  onPrepareRewind?: (turn: number, scope: RewindScope) => Promise<RewindPlan>;
  onCommitRewind?: (planId: string) => Promise<RewindResult>;
  onUndoRewind?: (transactionId: string) => Promise<void>;
}) {
  return (
    <div className="call" data-k="me" data-pending={item.pending ? "" : undefined}>
      <div className="g">
        <span className="sym">{t("你")}</span>
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className="nm">{t("我")}</span>
          {item.pending && <span className="pend">{t("排队中 · 下一个工具边界送达")}</span>}
          {/* The entry point lives on the turn it returns to, so there is no
              list to read and no turn number to match up by eye. */}
          {cp && onPrepareRewind && onCommitRewind && onUndoRewind && (
            <RewindControl cp={cp} onPrepare={onPrepareRewind} onCommit={onCommitRewind} onUndo={onUndoRewind} />
          )}
        </div>
        <div className="out">
          <div className="txt">{item.text}</div>
        </div>
      </div>
    </div>
  );
}
