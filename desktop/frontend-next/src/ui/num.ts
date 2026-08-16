import { useEffect, useRef, useState } from "react";

// Usage lands once per model round, so a counter that only ever cuts to its new
// value spends most of a turn looking frozen and then jumps. Easing the last
// leg — and flagging the moment it moved — is what makes the number read as
// live without inventing data the kernel never reported.

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

// True for one beat after value grows, so the spec's bump keyframes have
// something to fire on. Retriggering needs the attribute to leave the DOM
// first, which is why this is state and not a class toggle.
export function useBump(value: number, ms = 520): boolean {
  const [on, setOn] = useState(false);
  const prev = useRef(value);
  useEffect(() => {
    if (value <= prev.current) {
      prev.current = value;
      return;
    }
    prev.current = value;
    setOn(false);
    const raf = requestAnimationFrame(() => setOn(true));
    const t = setTimeout(() => setOn(false), ms);
    return () => {
      cancelAnimationFrame(raf);
      clearTimeout(t);
    };
  }, [value, ms]);
  return on;
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

// Whether the value moved recently at all — the difference between "receiving"
// and "stalled", which a still number cannot express on its own.
export function useFresh(value: number, ms = 1600): boolean {
  const [fresh, setFresh] = useState(false);
  const prev = useRef(value);
  useEffect(() => {
    if (value === prev.current) return;
    prev.current = value;
    setFresh(true);
    const t = setTimeout(() => setFresh(false), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return fresh;
}
