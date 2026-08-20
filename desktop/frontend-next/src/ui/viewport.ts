// How much room the interface actually has, published for the stylesheet to
// match on. A media query answers for the real viewport, and the zoom setting
// does not move it — at 1.3 everything is a third larger while the layout still
// believes it has the whole screen, so the columns never give way when they have
// run out of room. document.body is already laid out in the scaled space, so its
// content box is the viewport divided by the zoom with no arithmetic here.

// A fold is a column the interface gives up. They are cumulative by
// construction: anything narrow enough to drop the metrics column has already
// dropped the workspace one. The stylesheet names the fold, never the number.
const FOLDS = [
  { upTo: 1200, fold: "rail" }, // the workspace column goes
  { upTo: 840, fold: "side" }, // metrics moves under the flow
  { upTo: 640, fold: "scene" }, // the opening scene tightens
] as const;

/** foldsAt names every column given up at this width, widest fold first. */
export function foldsAt(width: number): string {
  return FOLDS.filter((f) => width <= f.upTo)
    .map((f) => f.fold)
    .join(" ");
}

const subs = new Set<(folds: string) => void>();

/** refresh re-reads the room and republishes it. Two things change it — the
 *  window resizing and the zoom setting — and each calls this rather than
 *  keeping its own copy of the thresholds. */
export function refresh() {
  const now = foldsAt(document.body.clientWidth);
  // Only on a real change: writing the attribute is a style invalidation, and an
  // unconditional write inside the observer is how a resize loop starts.
  if (document.documentElement.dataset.fold === now) return;
  document.documentElement.dataset.fold = now;
  subs.forEach((fn) => fn(now));
}

/** onFolds reports a change in what the layout has given up. A column that folds
 *  also has to stop being open, and open is state the stylesheet cannot reach. */
export function onFolds(fn: (folds: string) => void): () => void {
  subs.add(fn);
  return () => subs.delete(fn);
}

/** folded answers for right now — for state that has to be right on first paint,
 *  before any change has been reported. */
export function folded(name: string): boolean {
  return (document.documentElement.dataset.fold ?? "").split(" ").includes(name);
}

/** track publishes the width for the life of the page. */
export function track(): () => void {
  refresh();
  const ro = new ResizeObserver(refresh);
  ro.observe(document.body);
  return () => ro.disconnect();
}
