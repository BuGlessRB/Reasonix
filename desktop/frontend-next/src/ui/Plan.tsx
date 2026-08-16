import { useLayoutEffect, useRef, useState } from "react";
import { t } from "../i18n";
import type { PlanStep } from "../state/session";

// Keyed by text, not index: todo_write rewrites the whole list every call, and
// under index keys row one morphs into a different sentence instead of the old
// row leaving and a new one arriving. Duplicates get a suffix so React still
// sees one key per row.
function keys(steps: PlanStep[]): string[] {
  const seen = new Map<string, number>();
  return steps.map((s) => {
    const n = (seen.get(s.text) ?? 0) + 1;
    seen.set(s.text, n);
    return n === 1 ? s.text : `${s.text}#${n}`;
  });
}

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
          {t("计划")}<span className="c">0 / 0</span>
        </div>
        <div className="plan">
          <span className="empty">{t("尚未制定")}</span>
        </div>
      </div>
    );
  }

  const id = keys(steps);
  return (
    <div className="block" data-b="plan">
      <div className="lbl">
        {t("计划")}
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
            <div
              className="s"
              key={id[i]}
              data-done={st.done ? "" : undefined}
              data-now={i === now ? "" : undefined}
              style={{ animationDelay: `${Math.min(i, 6) * 34}ms` }}
            >
              <span className="b">{st.done ? "✓" : i + 1}</span>
              {/* The strike is painted as a background so it can be drawn rather
                  than appear, and an inline box is what makes it repeat per line
                  — on the flex item itself a wrapped step gets one line struck. */}
              <span className="t">
                <span className="ln">{st.text}</span>
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
