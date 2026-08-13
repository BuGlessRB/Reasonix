import { useState } from "react";
import type { Item } from "../../state/session";
import { Markdown } from "../Markdown";

export function SayCard({ item }: { item: Extract<Item, { t: "say" }> }) {
  const [open, setOpen] = useState(true);
  return (
    <div className="call" data-k="say">
      <div className="g">
        <span className="sym">◦</span>
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className="nm">Agent</span>
        </div>
        <div className="out">
          {item.reasoning && (
            <details className="think" open={open && !item.done} onToggle={(e) => setOpen(e.currentTarget.open)}>
              <summary>
                <span className="fold">
                  {item.done ? `想了 ${[...item.reasoning].length} 字` : "思考中…"}
                </span>
              </summary>
              <div className="tk">{item.reasoning}</div>
            </details>
          )}
          {item.text && (
            <div className="txt">
              <Markdown text={item.text} streaming={!item.done} />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
