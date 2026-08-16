import { useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { t } from "../i18n";

export interface RailMark {
  id: string;
  text: string;
  // Which block holds it and where inside that block, because the block is what
  // survives virtualisation: an unmounted one is a placeholder with the right
  // height, so its position is known even when the message itself has no node.
  block: number;
  within: number;
  of: number;
  files: number;
}

interface Props {
  marks: RailMark[];
  scroll: RefObject<HTMLDivElement | null>;
  flow: RefObject<HTMLDivElement | null>;
  onJump: (mark: RailMark) => void;
}

// Where a mark sits, in pixels down the content. Measured off the blocks rather
// than the messages: at any moment most of them are placeholders, and a
// placeholder holds the height its block had. Inside a block the position is
// interpolated — off by a card at worst, which a jump corrects by scrolling to
// the message itself once the block mounts.
function offsetsOf(root: HTMLElement, marks: RailMark[]): number[] {
  const chunks = root.querySelectorAll<HTMLElement>(".chunk");
  return marks.map((m) => {
    const chunk = chunks[m.block];
    if (!chunk) return 0;
    return chunk.offsetTop + (m.of > 1 ? (m.within / m.of) * chunk.offsetHeight : 0);
  });
}

export function Rail({ marks, scroll, flow, onJump }: Props) {
  const host = useRef<HTMLDivElement>(null);
  const [tops, setTops] = useState<number[]>([]);
  const [view, setView] = useState({ top: 0, height: 0 });
  const [at, setAt] = useState(-1);
  // Content height and rail height, read when they change rather than per
  // frame: a follow writes scrollTop every frame and must not pay a layout for
  // this on the way.
  const geom = useRef({ content: 1, rail: 1 });

  const measure = useCallback(() => {
    const root = scroll.current;
    const inner = flow.current;
    const box = host.current;
    if (!root || !inner || !box) return;
    const rail = root.clientHeight;
    box.style.setProperty("--rail-h", rail + "px");
    geom.current = { content: root.scrollHeight || 1, rail };
    setTops(offsetsOf(inner, marks).map((y) => (y / (root.scrollHeight || 1)) * rail));
    setView({
      top: (root.scrollTop / (root.scrollHeight || 1)) * rail,
      height: Math.max(16, (root.clientHeight / (root.scrollHeight || 1)) * rail),
    });
  }, [marks, scroll, flow]);

  useLayoutEffect(measure, [measure]);

  useEffect(() => {
    const root = scroll.current;
    const inner = flow.current;
    if (!root || !inner) return;
    // Scrolling only moves the viewport box, and that needs no measurement —
    // the two heights it divides by were captured when they last changed.
    const onScroll = () => {
      const { content, rail } = geom.current;
      setView((v) => ({ ...v, top: (root.scrollTop / content) * rail }));
    };
    root.addEventListener("scroll", onScroll, { passive: true });
    const ro = new ResizeObserver(measure);
    ro.observe(inner);
    ro.observe(root);
    return () => {
      root.removeEventListener("scroll", onScroll);
      ro.disconnect();
    };
  }, [scroll, flow, measure]);

  // The pointer's y decides which mark it means. Hitting a three-pixel line is
  // not a thing anyone should have to do, and a mark that moved under the
  // pointer to acknowledge the hover would take itself out from under it.
  const aim = (y: number) => {
    let best = -1;
    let dist = Infinity;
    tops.forEach((top, i) => {
      const d = Math.abs(top - y);
      if (d < dist) {
        dist = d;
        best = i;
      }
    });
    setAt(dist <= 120 ? best : -1);
  };

  if (marks.length === 0) return null;
  const shown = at >= 0 ? marks[at] : null;

  return (
    <div className="railhost" ref={host} aria-hidden={undefined}>
      <nav
        className="srail"
        aria-label={t("你说过的话")}
        onMouseMove={(e) => aim(e.clientY - e.currentTarget.getBoundingClientRect().top)}
        onMouseLeave={() => setAt(-1)}
        onClick={(e) => {
          if (e.target !== e.currentTarget) return;
          const i = at;
          if (i >= 0) onJump(marks[i]);
        }}
      >
        <i className="srail-view" style={{ top: view.top, height: view.height }} />
        {marks.map((m, i) => (
          <button
            key={m.id}
            className="srail-m"
            style={{ top: tops[i] ?? 0 }}
            data-on={i === at ? "" : undefined}
            data-close={at >= 0 && Math.abs(i - at) === 1 && Math.abs((tops[i] ?? 0) - (tops[at] ?? 0)) < 46 ? "" : undefined}
            aria-label={m.text.slice(0, 40)}
            onFocus={() => setAt(i)}
            onBlur={() => setAt(-1)}
            onClick={() => onJump(m)}
          >
            {/* 会动的是这条线，按钮本身不动 —— 它一动就会从指针底下跑掉 */}
            <i style={{ height: Math.min(3 + m.files * 1.6, 9), width: Math.min(10 + m.files * 3, 26) }} />
          </button>
        ))}
      </nav>
      {/* 贴着轨道两端的那几条，预览要收进容器里 —— 否则它会被裁掉一半 */}
      {shown && (
        <div
          className="srail-peek"
          style={{ top: Math.min(Math.max(tops[at] ?? 0, 44), Math.max(44, geom.current.rail - 44)) }}
          role="tooltip"
        >
          <div className="hd">
            <span className="n">{t("第 {n} 句", { n: at + 1 })}</span>
            {shown.files > 0 && <span>{t("{n} 个文件", { n: shown.files })}</span>}
          </div>
          <div className="tx">{shown.text}</div>
        </div>
      )}
    </div>
  );
}
