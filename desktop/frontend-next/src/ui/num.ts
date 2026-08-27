import { useEffect, useRef, useState } from "react";

// Usage lands once per model round, so a counter that only ever cuts to its new
// value spends most of a turn looking frozen and then jumps. Easing the last
// leg is what makes the number read as live without inventing data the kernel
// never reported.

const EASE = (t: number) => 1 - Math.pow(1 - t, 3);

export function useTicker(value: number, ms = 520): number {
  const [shown, setShown] = useState(value);
  const from = useRef(value);
  const raf = useRef(0);
  useEffect(() => {
    const start = performance.now();
    const a = from.current;
    if (a === value) return;
    const step = (now: number) => {
      const t = Math.min(1, (now - start) / ms);
      const at = a + (value - a) * EASE(t);
      setShown(t === 1 ? value : at);
      from.current = at;
      if (t < 1) raf.current = requestAnimationFrame(step);
    };
    raf.current = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf.current);
  }, [value, ms]);
  return shown;
}

// One sample a second while the turn runs, capped at `span`. A rate is the one
// figure on the rail whose shape says more than its current value — whether it
// is climbing, stalling or ragged — and a single number cannot hold that. The
// timer stops when the turn does: an idle rail must not tick.
export function useTrail(value: number, on: boolean, span = 60): number[] {
  const [trail, setTrail] = useState<number[]>([]);
  const cur = useRef(value);
  cur.current = value;
  useEffect(() => {
    if (!on) return;
    const id = setInterval(() => setTrail((t) => [...t, cur.current].slice(-span)), 1000);
    return () => clearInterval(id);
  }, [on, span]);
  return trail;
}
