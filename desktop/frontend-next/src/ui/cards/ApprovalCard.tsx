import type { Item } from "../../state/session";
import type { ApprovalVerdict } from "../../port/port";

interface Props {
  item: Extract<Item, { t: "approval" }>;
  onApprove: (itemId: string, id: string, v: ApprovalVerdict) => void;
}

// The run is genuinely blocked here until Approve() resolves it, so this card
// must be the only way past — no other control may advance the queue.
export function ApprovalCard({ item, onApprove }: Props) {
  const sealed = item.verdict !== undefined;
  return (
    <div className="call" data-k="ask">
      <div className="g">
        <span className="sym">?</span>
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className="nm">要动手了</span>
          <span className="tag">approval_request</span>
        </div>
        <div className="out">
          <div className="apv" data-sealed={sealed ? item.verdict : undefined}>
            <div className="apv-hd">
              <span className="tool">{item.a.tool}</span>
              <span className="sub">{item.a.subject}</span>
            </div>
            {item.a.reason && <div className="apv-dt">{item.a.reason}</div>}
            {!sealed && (
              <div className="apv-ft">
                <button className="btn" data-primary onClick={() => onApprove(item.id, item.a.id, "once")}>
                  允许这一次
                </button>
                <button className="btn" onClick={() => onApprove(item.id, item.a.id, "always")}>
                  这一类不再问
                </button>
                <button className="btn" onClick={() => onApprove(item.id, item.a.id, "deny")}>
                  拒绝
                </button>
              </div>
            )}
            {sealed && (
              <div className="apv-done">
                {item.verdict === "always" ? (
                  <><b>本会话不再问这一类。</b>核心把它记进会话授权，不落盘。</>
                ) : item.verdict === "deny" ? (
                  <><b>已拒绝。</b>agent 收到否决，会另想办法或停手。</>
                ) : (
                  <><b>允许这一次。</b>下次同样的操作仍会问你。</>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
