import { useLayoutEffect, useRef, useState, type RefObject } from "react";

// The row's children laid end to end, gaps included. Auto margins are left out
// on purpose: they are free space, not something the labels need.
function natural(row: HTMLElement) {
  const kids = [...row.children] as HTMLElement[];
  const gap = parseFloat(getComputedStyle(row).columnGap) || 0;
  return kids.reduce((sum, el) => sum + el.offsetWidth, 0) + gap * Math.max(0, kids.length - 1);
}

/** useLabelFit reports whether a row of labelled controls has to fall back to
 *  icons. The row must not size itself from its content (the stylesheet gives
 *  it `contain: inline-size`), so the room it has is its own width. The width
 *  the labels need is only readable while they are drawn, so it is kept from
 *  the last labelled layout; `labels` names what is drawn, and a change to it
 *  measures again. */
export function useLabelFit(row: RefObject<HTMLElement | null>, labels: string): boolean {
  const [compact, setCompact] = useState(false);
  const need = useRef(0);

  useLayoutEffect(() => {
    need.current = 0;
    setCompact(false);
  }, [labels]);

  useLayoutEffect(() => {
    const el = row.current;
    if (!el) return;
    const settle = () => {
      if (!compact) need.current = natural(el);
      const fits = need.current <= el.clientWidth;
      if (fits === compact) setCompact(!fits);
    };
    settle();
    const ro = new ResizeObserver(settle);
    ro.observe(el);
    return () => ro.disconnect();
  }, [row, compact, labels]);

  return compact;
}
