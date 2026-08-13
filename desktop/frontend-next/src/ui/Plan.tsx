import { useLayoutEffect, useRef, useState } from "react";
import type { PlanStep } from "../state/session";

export function Plan({ steps }: { steps: PlanStep[] }) {
  const wrap = useRef<HTMLDivElement>(null);
  const [cursor, setCursor] = useState<{ y: number; h: number } | null>(null);
  const now = steps.findIndex((s) => !s.done);
  const done = steps.filter((s) => s.done).length;

  // The cursor is measured from the live row, so it survives text wrapping and
  // font changes that a fixed row height would get wrong.
  useLayoutEffect(() => {
    const el = wrap.current?.querySelector<HTMLElement>('.s[data-now]');
    setCursor(el ? { y: el.offsetTop + 6, h: Math.max(0, el.offsetHeight - 11) } : null);
  }, [steps, now]);

  if (steps.length === 0) {
    return (
      <div className="block" data-b="plan">
        <div className="lbl">
          计划<span className="c">0 / 0</span>
        </div>
        <div className="plan">
          <span className="empty">尚未制定</span>
        </div>
      </div>
    );
  }

  return (
    <div className="block" data-b="plan">
      <div className="lbl">
        计划
        <span className="c">
          {done} / {steps.length}
        </span>
      </div>
      <div className="prog2">
        <span className="track">
          <i style={{ width: `${Math.round((done / steps.length) * 100)}%` }} />
        </span>
      </div>
      <div className="planwrap" ref={wrap}>
        {cursor && <i className="cursor" style={{ height: cursor.h, transform: `translateY(${cursor.y}px)` }} />}
        <div className="plan">
          {steps.map((st, i) => (
            <div className="s" key={i} data-done={st.done ? "" : undefined} data-now={i === now ? "" : undefined}>
              <span className="b">{st.done ? "✓" : i + 1}</span>
              <span className="t">{st.text}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
