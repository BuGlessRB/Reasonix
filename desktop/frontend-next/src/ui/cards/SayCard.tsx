import { useState } from "react";
import { Sym } from "../Sym";
import type { Item } from "../../state/session";
import { Markdown } from "../Markdown";
import { Boundary } from "../Boundary";
import { useRevealed } from "../reveal";

export function SayCard({ item }: { item: Extract<Item, { t: "say" }> }) {
  const [open, setOpen] = useState(true);
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
              <div className="tk">
                {thought}
                {!item.done && <span className="caret" />}
              </div>
            </details>
          )}
          {item.text && (
            <div className="txt">
              <Boundary fallback={<div className="md">{item.text}</div>}>
                <Markdown text={item.text} streaming={!item.done} />
              </Boundary>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
