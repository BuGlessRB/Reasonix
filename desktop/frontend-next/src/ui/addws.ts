import { useCallback, useMemo, useState } from "react";
import type { HubPort } from "../port/hub";

export interface Adder {
  add: () => void;
  busy: boolean;
}

/** useAddWorkspace is the one implementation of "open a project", so the
 *  sidebar's entry and the first-run banner cannot drift into two behaviours. */
export function useAddWorkspace(hub: HubPort, reload: () => Promise<void>, onError: (e: unknown) => void): Adder {
  const [busy, setBusy] = useState(false);

  const add = useCallback(
    () => {
      if (busy) return;
      setBusy(true);
      void hub
        .pickFolder()
        .then(async (dir) => {
          if (dir === null) {
            onError(new Error("浏览器预览无法读取本地文件夹路径，请在 Reasonix Studio 桌面端选择工作区。"));
            return;
          }
          // "" is the user closing the panel — an answer, not a reason to ask again.
          if (!dir) return;
          await hub.addWorkspace(dir);
          await reload();
        })
        .catch(onError)
        .finally(() => setBusy(false));
    },
    [busy, hub, reload, onError],
  );

  return useMemo(() => ({ add, busy }), [add, busy]);
}
