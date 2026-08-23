// Placing a fixed element while the interface is zoomed puts two coordinate
// systems into one calculation. Measured in Chromium with html{zoom:1.5}: a
// 232px box reports 348 wide, an element at left:400 reports 600, and writing
// left:300 lands it at 450. So a client rect and the viewport are both read on
// screen, while a left written back is multiplied by the zoom on its way in.
// The arithmetic stays on screen and divides once at the end — the same
// correction the stylesheet makes when it writes calc(100vh / var(--zoom, 1)).

/** zoom is the interface scale look.ts publishes; 1 when it is not scaled. */
export function zoom(): number {
  const raw = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--zoom"));
  return raw > 0 ? raw : 1;
}

export interface Size {
  width: number;
  height: number;
}

/** placeInViewport is the arithmetic, apart from the DOM so it can be checked
 *  without a browser. want, box and viewport are all on-screen values; the
 *  result is what goes into the style, which the zoom scales up again. */
export function placeInViewport(
  want: { x: number; y: number },
  box: Size,
  viewport: Size,
  scale: number,
  edge = 6,
): { left: number; top: number } {
  const left = Math.max(edge, Math.min(want.x, viewport.width - box.width - edge));
  const top = Math.max(edge, Math.min(want.y, viewport.height - box.height - edge));
  return { left: left / scale, top: top / scale };
}

/** pinToViewport puts a fixed element's top-left at (x, y) — a point in the
 *  space getBoundingClientRect reports — and keeps the whole box inside the
 *  viewport, so a bubble anchored near an edge is moved rather than clipped. */
export function pinToViewport(el: HTMLElement, x: number, y: number, edge = 6): void {
  const box = el.getBoundingClientRect();
  const at = placeInViewport({ x, y }, box, { width: innerWidth, height: innerHeight }, zoom(), edge);
  el.style.left = `${at.left}px`;
  el.style.top = `${at.top}px`;
}
