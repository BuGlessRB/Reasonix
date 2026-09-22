import { useEffect } from "react";
import type { AgentPort } from "../port/port";
import { host } from "../port/host";

/** Where a link in this window goes. A webview has nowhere to put a new tab,
 *  so target="_blank" opens nothing at all, and letting the link navigate in
 *  place would replace the session with the page.
 *
 *  It goes to the browser this window draws, which is also the one the model
 *  can read and drive: a page opened outside is a page nobody in the
 *  conversation can see. A modifier is the way out to the machine's own
 *  browser, and a shell that draws no views has only that way. */
export function useLinkRouting(port: AgentPort | null, reveal: () => void, onError: (e: unknown) => void) {
  useEffect(() => {
    if (!port) return;
    const onClick = (e: MouseEvent) => {
      if (e.defaultPrevented || e.button !== 0) return;
      const link = (e.target as Element | null)?.closest?.("a[href]");
      const href = link?.getAttribute("href") ?? "";
      if (!/^https?:\/\//i.test(href)) return;
      e.preventDefault();
      if (e.ctrlKey || e.metaKey || e.shiftKey || !host().drawsBrowserViews()) {
        void port.openExternal(href).catch(onError);
        return;
      }
      reveal();
      // A kernel that cannot open one still has somewhere to send it, rather
      // than a click that did nothing.
      void port.browserOpen(href, true).catch(() => port.openExternal(href).catch(onError));
    };
    addEventListener("click", onClick);
    return () => removeEventListener("click", onClick);
  }, [port, reveal, onError]);
}
