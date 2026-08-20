import { useCallback, useMemo, useState } from "react";
import type { HubPort } from "../port/hub";

export interface Adder {
  // from names the entry that asked, and typing answers with it. Both entries
  // share this state, so a plain flag opened a field under each of them — and
  // the second one's autoFocus blurred the first, which closed both.
  add: (from: string) => void;
  addPath: (path: string) => void;
  // The native panel is the main path; typing one is the escape hatch. A browser
  // tab never learns a real path, and a shell whose panel refuses to open would
  // otherwise leave no way in at all.
  typing: string;
  setTyping: (from: string) => void;
  busy: boolean;
}

/** useAddWorkspace is the one implementation of "open a project", so the
 *  sidebar's entry and the first-run banner cannot drift into two behaviours. */
export function useAddWorkspace(hub: HubPort, reload: () => Promise<void>, onError: (e: unknown) => void): Adder {
  const [busy, setBusy] = useState(false);
  const [typing, setTyping] = useState("");

  const addPath = useCallback(
    (path: string) => {
      if (!path.trim()) return;
      setBusy(true);
      void hub
        .addWorkspace(path.trim())
        .then(reload)
        .catch(onError)
        .finally(() => setBusy(false));
    },
    [hub, reload, onError],
  );

  const add = useCallback(
    (from: string) => {
      setBusy(true);
      void hub
        .pickFolder()
        .then(async (dir) => {
          if (dir === null) {
            setTyping(from);
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
    [hub, reload, onError],
  );

  return useMemo(() => ({ add, addPath, typing, setTyping, busy }), [add, addPath, typing, busy]);
}
