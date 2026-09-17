import { useEffect, useLayoutEffect, useRef, useState, type FormEvent } from "react";
import { t } from "../i18n";
import type { AgentPort, BrowserTab } from "../port/port";
import { host, type BrowserControl, type ViewRect } from "../port/host";
import { SWAP_MARK } from "./swap";

/** The agent's open pages, read back whenever the kernel says they moved. A
 *  shell that cannot draw them answers none, so nothing offers a view of them. */
export function useBrowserTabs(port: AgentPort, moved: number): BrowserTab[] {
  const [tabs, setTabs] = useState<BrowserTab[]>([]);
  useEffect(() => {
    if (!host().drawsBrowserViews()) return;
    let live = true;
    port.browserTabs().then((next) => live && setTabs(next), () => {});
    return () => {
      live = false;
    };
  }, [port, moved]);
  return tabs;
}

type Probe = (x: number, y: number) => Element | null;

/** Whether anything is drawn over the placeholder. The page is drawn above by a
 *  view the DOM cannot see, so a menu or a sheet on top of this rectangle would
 *  be hidden under it: sampling what the page itself shows at a few points is
 *  the one answer that does not need every overlay to announce itself. */
export function covered(slot: Element, rect: ViewRect, probe: Probe): boolean {
  for (const fx of [0.08, 0.5, 0.92]) {
    for (const fy of [0.08, 0.5, 0.92]) {
      const hit = probe(rect.x + rect.width * fx, rect.y + rect.height * fy);
      if (hit !== slot && !(hit && slot.contains(hit))) return true;
    }
  }
  return false;
}

export function BrowserPanel({ tabs, shown }: { tabs: BrowserTab[]; shown: boolean }) {
  const slot = useRef<HTMLDivElement>(null);
  const [picked, setPicked] = useState("");
  const current = tabs.find((tab) => tab.target === picked) ?? tabs.find((tab) => tab.active) ?? tabs[0];
  const [address, setAddress] = useState(current?.url ?? "");
  const target = current?.target ?? "";

  useEffect(() => setAddress(current?.url ?? ""), [current?.url]);

  useLayoutEffect(() => {
    const el = slot.current;
    const shell = host();
    if (!el || !shown || !target) {
      shell.hideBrowserView();
      return;
    }
    let frame = 0;
    const place = () => {
      frame = 0;
      const box = el.getBoundingClientRect();
      const rect = { x: box.left, y: box.top, width: box.width, height: box.height };
      if (rect.width < 1 || rect.height < 1 || covered(el, rect, (x, y) => document.elementFromPoint(x, y))) {
        shell.hideBrowserView();
      } else {
        shell.showBrowserView(target, rect);
      }
    };
    // A timer, not an animation frame: a window the system counts as hidden runs
    // no frames, and the page would never be drawn once it is uncovered.
    const schedule = () => {
      if (!frame) frame = window.setTimeout(place, 16);
    };
    place();
    const resize = new ResizeObserver(schedule);
    resize.observe(el);
    const overlays = new MutationObserver(schedule);
    overlays.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ["hidden", "class", "style", "open", "data-prefs"] });
    // A view transition covers the whole page while it runs, and its end changes
    // nothing in the body: only the mark on the root says it is over.
    overlays.observe(document.documentElement, { attributes: true, attributeFilter: [SWAP_MARK] });
    addEventListener("resize", schedule);
    return () => {
      if (frame) clearTimeout(frame);
      resize.disconnect();
      overlays.disconnect();
      removeEventListener("resize", schedule);
      shell.hideBrowserView();
    };
  }, [shown, target]);

  const control = (action: BrowserControl) => target && host().controlBrowserView(target, action);
  const go = (event: FormEvent) => {
    event.preventDefault();
    if (target) void host().navigateBrowserView(target, address);
  };

  return (
    <div className="bpanel">
      <div className="bbar">
        <div className="vtabs" role="tablist">
          {tabs.map((tab) => (
            <button
              key={tab.target}
              className="tab btab" role="tab" data-action="browser.tab" data-target={tab.id}
              aria-selected={tab.target === target} title={tab.url}
              onClick={() => setPicked(tab.target)}
            >
              {tab.title || tab.url || t("空白页")}
            </button>
          ))}
        </div>
        <div className="bnav">
          <button className="bbtn" data-action="browser.control" data-target={current?.id} data-value="back" aria-label={t("后退")} onClick={() => control("back")}>←</button>
          <button className="bbtn" data-action="browser.control" data-target={current?.id} data-value="forward" aria-label={t("前进")} onClick={() => control("forward")}>→</button>
          <button className="bbtn" data-action="browser.control" data-target={current?.id} data-value="reload" aria-label={t("重新加载")} onClick={() => control("reload")}>↻</button>
          <form className="baddr" data-action="browser.navigate" data-target={current?.id} onSubmit={go}>
            <input
              data-action="browser.navigate" data-target={current?.id}
              value={address} spellCheck={false} aria-label={t("网址")}
              onChange={(event) => setAddress(event.target.value)}
            />
          </form>
        </div>
      </div>
      <div className="bview" ref={slot}>
        <p className="bhint">{t("页面被遮住时暂停显示")}</p>
      </div>
    </div>
  );
}
