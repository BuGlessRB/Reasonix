import { useEffect, useRef, useState } from "react";

interface Props {
  // full plays the whole sequence; short stops after the introduction, for a
  // machine that already has a key and nothing to ask for.
  variant: "full" | "short";
  onDone: () => void;
}

// The beat the collapse happens on. The card fades in just after it, so the
// brand never leaves the screen between the introduction and the form.
const COLLAPSE_MS = 9600;
const SHORT_MS = 6200;

// Welcome is the opening sequence: aurora on a fixed dark ground, the R drawn
// and then traced by a light, the wordmark swept by a highlight, three lines
// of introduction, and a collapse that turns the whole group into the header
// of whatever comes next. It plays once per machine.
//
// The ground is dark whatever the app theme is. This is a ceremony rather than
// a screen of the app, the way an OS out-of-box sequence does not follow the
// user's colour scheme.
export function Welcome({ variant, onDone }: Props) {
  const [leaving, setLeaving] = useState(false);
  const done = useRef(false);

  // One exit for every route out: the timer, a key, a click, or reduced motion.
  useEffect(() => {
    const finish = () => {
      if (done.current) return;
      done.current = true;
      setLeaving(true);
      window.setTimeout(onDone, variant === "full" ? 0 : 380);
    };
    const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const at = reduced ? 0 : variant === "full" ? COLLAPSE_MS : SHORT_MS;
    const timer = window.setTimeout(finish, at);
    const skip = () => finish();
    window.addEventListener("keydown", skip);
    window.addEventListener("pointerdown", skip);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("keydown", skip);
      window.removeEventListener("pointerdown", skip);
    };
  }, [variant, onDone]);

  return (
    <div className="oobe" data-play={leaving ? undefined : ""} data-leaving={leaving ? "" : undefined}>
      <div className="oobe-glow g1" />
      <div className="oobe-glow g2" />
      <div className="oobe-glow g3" />
      <div className="oobe-vig" />

      <div className="oobe-scene">
        <svg className="oobe-mark" viewBox="0 0 16 16" aria-hidden="true">
          <path className="base" pathLength={100} d="M4.7 2.9V13.1" />
          <path className="base" pathLength={100} d="M4.7 2.9h4.2a2.9 2.9 0 0 1 0 5.8H4.7" />
          <path className="base" pathLength={100} d="M9 8.7l3.3 4.4" />
          <path className="lume" pathLength={100} d="M4.7 13.1V2.9h4.2a2.9 2.9 0 0 1 0 5.8H4.7l3.6 4.4" />
        </svg>
        <h1 className="oobe-brand">Reasonix Studio</h1>
        <p className="oobe-sub">coding agent</p>

        <div className="oobe-lines" aria-live="polite">
          <div className="oobe-line l1">
            <b>我是小 R。</b>
          </div>
          <div className="oobe-line l2">
            <b>你交待一件事，我把它做完。</b>
            <span>读代码、查证、动手、验证 —— 不是给你一段建议就完了。</span>
          </div>
          <div className="oobe-line l3">
            <b>每一步都留得下来。</b>
            <span>改了哪个文件、跑了哪条命令、花了多少 token，都能翻回去看。</span>
          </div>
        </div>
      </div>

      <div className="oobe-prog"><i /></div>
      <span className="oobe-skip">按任意键跳过</span>
    </div>
  );
}
