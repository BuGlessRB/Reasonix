import { useEffect, useState } from "react";
import { t } from "../i18n";

// Wails publishes bound methods at window.go.<package>.<Struct>.<Method>.
interface Shell {
  go?: {
    main?: {
      App?: {
        MinimiseWindow?: () => Promise<void>;
        ToggleMaximiseWindow?: () => Promise<void>;
        IsWindowMaximised?: () => Promise<boolean>;
        CloseWindow?: () => Promise<void>;
      };
    };
  };
}

const app = () => (window as unknown as Shell).go?.main?.App;

// Where the chrome is standing in for the title bar. Linux keeps its own frame,
// and a browser tab has no window to zoom.
const isTitleBar = () => {
  const d = document.documentElement.dataset;
  return d.shell === "wails" && (d.platform === "darwin" || d.platform === "windows");
};

// Double-clicking a title bar zooms the window on both platforms. The drag
// region is drawn by the webview, so nothing native is left to hear the click.
export function zoomOnTitleBar(e: { target: EventTarget | null }) {
  if (!isTitleBar()) return;
  const el = e.target as HTMLElement | null;
  if (el?.closest("button, input, textarea, .picker, .menu")) return;
  void app()?.ToggleMaximiseWindow?.();
}

// Frameless Windows has no native minimise/maximise/close, so a shell that goes
// frameless without drawing these leaves a window that can only be closed from
// the keyboard. Rendered on Windows alone: macOS keeps its own lights and Linux
// its whole frame.
export function WindowControls() {
  const [max, setMax] = useState(false);
  useEffect(() => {
    app()?.IsWindowMaximised?.().then(setMax).catch(() => {});
  }, []);
  if (document.documentElement.dataset.platform !== "windows") return null;
  return (
    <div className="winctl" role="group" aria-label="窗口">
      <button className="wc" onClick={() => void app()?.MinimiseWindow?.()} aria-label="最小化">
        <svg viewBox="0 0 12 12" aria-hidden="true">
          <path d="M2 6h8" />
        </svg>
      </button>
      <button
        className="wc"
        onClick={() => {
          void app()?.ToggleMaximiseWindow?.();
          app()?.IsWindowMaximised?.().then(setMax).catch(() => {});
        }}
        aria-label={t(max ? "还原" : "最大化")}
      >
        <svg viewBox="0 0 12 12" aria-hidden="true">
          {max ? (
            <>
              <path d="M3.4 3.4V2.2h6.4v6.4H8.6" />
              <path d="M2.2 3.4h6.4v6.4H2.2Z" />
            </>
          ) : (
            <path d="M2.4 2.4h7.2v7.2H2.4Z" />
          )}
        </svg>
      </button>
      <button className="wc close" onClick={() => void app()?.CloseWindow?.()} aria-label="关闭">
        <svg viewBox="0 0 12 12" aria-hidden="true">
          <path d="M2.6 2.6l6.8 6.8M9.4 2.6l-6.8 6.8" />
        </svg>
      </button>
    </div>
  );
}
