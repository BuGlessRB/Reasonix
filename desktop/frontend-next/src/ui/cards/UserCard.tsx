import type { Item } from "../../state/session";

export function UserCard({ item }: { item: Extract<Item, { t: "user" }> }) {
  return (
    <div className="call" data-k="me" data-pending={item.pending ? "" : undefined}>
      <div className="g">
        <span className="sym">你</span>
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className="nm">我</span>
          {item.pending && <span className="pend">排队中 · 下一个工具边界送达</span>}
        </div>
        <div className="out">
          <div className="txt">{item.text}</div>
        </div>
      </div>
    </div>
  );
}
