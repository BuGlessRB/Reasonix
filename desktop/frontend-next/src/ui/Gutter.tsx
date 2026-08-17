import { useRef } from "react";
import type { PointerEvent as ReactPointerEvent, KeyboardEvent as ReactKeyboardEvent } from "react";

export interface Span {
  min: number;
  max: number;
  def: number;
  key: string;
  css: string;
}

// 拖出来的宽度是读者对自己这块屏幕的判断，不是布局的默认值 —— 所以它归本地
// 存，不进内核配置：换台机器、换块屏幕，判断本来就不同。
export const RAIL: Span = { min: 168, max: 420, def: 214, key: "rx-rail-w", css: "--rail-open" };
export const SIDE: Span = { min: 232, max: 520, def: 296, key: "rx-side-w", css: "--side-open" };

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));

export function widthOf(span: Span): number {
  const saved = Number(localStorage.getItem(span.key));
  return Number.isFinite(saved) && saved > 0 ? clamp(saved, span.min, span.max) : span.def;
}

interface Props {
  edge: "l" | "r";
  span: Span;
  width: number;
  label: string;
  onWidth: (w: number) => void;
}

export function Gutter({ edge, span, width, label, onWidth }: Props) {
  const held = useRef(0);

  const drag = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return;
    const bar = e.currentTarget;
    const app = bar.closest(".app") as HTMLElement | null;
    if (!app) return;
    e.preventDefault();

    const x0 = e.clientX;
    let now = width;
    let raf = 0;
    bar.setPointerCapture(e.pointerId);
    bar.dataset.on = "";
    app.dataset.drag = "pointer";

    // 每帧把宽度直接写进变量，而不是走一次 setState：栏宽是拖着的手在感觉的
    // 东西，而这棵树上还挂着一条正在流的转录。每帧重写一次也是为了压过中途
    // 的重渲染 —— 流式期间 React 会把这个 style 刷回状态里的旧值。
    const paint = () => {
      app.style.setProperty(span.css, `${now}px`);
      raf = requestAnimationFrame(paint);
    };
    const move = (ev: PointerEvent) => {
      now = clamp(width + (edge === "l" ? ev.clientX - x0 : x0 - ev.clientX), span.min, span.max);
    };
    const up = () => {
      cancelAnimationFrame(raf);
      bar.removeEventListener("pointermove", move);
      bar.removeEventListener("pointerup", up);
      bar.removeEventListener("pointercancel", up);
      delete bar.dataset.on;
      // 最后一次 move 之后未必还有一帧，栏会停在比松手位置差几像素的地方。这
      // 几像素得在补间恢复之前就落定，否则它们会被慢慢走完 —— 松手时的一记回
      // 弹。读一次布局，就是让这个值先以「不补间」的身份定下来。
      app.style.setProperty(span.css, `${now}px`);
      void app.offsetWidth;
      delete app.dataset.drag;
      onWidth(now);
    };
    bar.addEventListener("pointermove", move);
    bar.addEventListener("pointerup", up);
    bar.addEventListener("pointercancel", up);
    paint();
  };

  const key = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    const dir = e.key === "ArrowLeft" ? -1 : e.key === "ArrowRight" ? 1 : 0;
    if (dir) {
      e.preventDefault();
      // 一步 12px 配一段 .34s 的补间，连按就成了追不上的橡皮筋。按住时不补间，
      // 停手后补间归位 —— 双击复位那种整段位移仍然该被看见。
      const app = e.currentTarget.closest(".app") as HTMLElement | null;
      if (app) {
        app.dataset.drag = "key";
        clearTimeout(held.current);
        held.current = window.setTimeout(() => delete app.dataset.drag, 220);
      }
      onWidth(clamp(width + dir * (e.shiftKey ? 48 : 12) * (edge === "l" ? 1 : -1), span.min, span.max));
    } else if (e.key === "Home") {
      e.preventDefault();
      onWidth(span.def);
    }
  };

  return (
    <div
      className={`gutter gutter-${edge}`}
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuenow={Math.round(width)}
      aria-valuemin={span.min}
      aria-valuemax={span.max}
      tabIndex={0}
      onPointerDown={drag}
      onDoubleClick={() => onWidth(span.def)}
      onKeyDown={key}
    />
  );
}
