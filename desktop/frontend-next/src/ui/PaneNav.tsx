import { useRef } from "react";
import { t } from "../i18n";
import { arrowTabs } from "./tablist";

export type PaneView = "flow" | "analysis" | "browser";

/** The pane's own navigation. It projects the five views onto three standing
 *  purposes; it does not own them, and it never holds a selection of its own —
 *  a second piece of state saying which detail is open is a second answer to a
 *  question the pane has already answered. */
export function PaneNav({ view, onPick, rows, pages, dock, onDock }: {
  view: PaneView;
  onPick: (to: PaneView) => void;
  rows: number;
  // The agent's open browser pages. The tab exists only while there is one.
  pages: number;
  // Whether those pages sit beside the conversation, and the switch for it.
  dock: boolean;
  onDock: () => void;
}) {
  const bar = useRef<HTMLDivElement>(null);
  // Read at render: t() answers out of a dictionary boot() installs, and a
  // table built in the module body freezes the labels in the source language.
  const name: Record<PaneView, string> = {
    flow: t("对话"), analysis: t("运行分析"), browser: t("浏览器"),
  };

  return (
    <div className="tabs">
      <div className="vtabs" role="tablist" ref={bar} onKeyDown={arrowTabs}>
        {/* No item count. How many rows a transcript holds does not change what
            anyone does next, and it was the largest number on the bar. */}
        <button
          className="tab" role="tab" data-action="pane.view" data-value="flow"
          aria-selected={view === "flow"} onClick={() => onPick("flow")}
        >
          {name.flow}
        </button>
        {rows > 0 && (
          <button
            className="tab" role="tab" data-action="pane.view" data-value="analysis"
            aria-selected={view === "analysis"} onClick={() => onPick("analysis")}
          >
            {name.analysis}
          </button>
        )}
        {pages > 0 && (
          <button
            className="tab" role="tab" data-action="pane.view" data-value="browser"
            aria-selected={view === "browser"} onClick={() => onPick("browser")}
          >
            {name.browser}
            <span className="n">{pages}</span>
          </button>
        )}
        {/* Beside, not instead: watching the agent browse and reading what it
            says are one activity. Drawn only where there is a page for it. */}
        {pages > 0 && (
          <button
            className="tab dock" data-action="pane.dock" data-value={dock ? "off" : "on"}
            aria-pressed={dock} title={t("并排显示浏览器")} aria-label={t("并排显示浏览器")}
            onClick={onDock}
          >
            ⿲
          </button>
        )}
      </div>
    </div>
  );
}
