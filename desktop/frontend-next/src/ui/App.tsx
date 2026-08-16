import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { reason } from "../i18n/kernel";
import { t } from "../i18n";
import type { AccountState, AgentPort, Appearance as Look, ProviderSetup, ThemePack } from "../port/port";
import type { HubPort, RuntimeView, TreeWorkspace } from "../port/hub";
import { Chrome } from "./Chrome";
import { apply as applyThemePack } from "./theme";
import { apply as applyLook } from "./look";
import { adopt as adoptLang } from "../i18n";
import { Pane, type PaneReport } from "./Pane";
import { Workspaces } from "./Workspaces";
import { PaneTabs } from "./PaneTabs";
import { Settings } from "./Settings";
import { Onboarding } from "./Onboarding";
import { Welcome } from "./Welcome";

const NO_REPORT: PaneReport = { status: null, title: "", steer: 0, run: "idle", live: false, cost: "" };

// App is the window around the panes, not a session itself: the workspace tree,
// the chrome, the settings sheet and the theme are the window's, while every
// conversation — its transcript, metrics and event stream — lives in the Pane
// that owns it. That split is what lets two sessions run side by side.
export function App({ hub }: { hub: HubPort }) {
  const [runtimes, setRuntimes] = useState<RuntimeView[]>([]);
  const [active, setActive] = useState("");
  const [tree, setTree] = useState<TreeWorkspace[]>([]);
  const [folded, setFolded] = useState<Set<string>>(new Set());
  const [rail, setRail] = useState(true);
  const [side, setSide] = useState(true);
  const [report, setReport] = useState<PaneReport>(NO_REPORT);
  const [error, setError] = useState("");
  // false = closed, true = open at its last section, a string = open there.
  const [settings, setSettings] = useState<string | boolean>(false);
  const [theme, setTheme] = useState(() => localStorage.getItem("rx-theme") ?? "auto");
  // "" means never chosen, which is what lets the system's own contrast setting
  // decide. Any explicit pick wins over it from then on.
  const [contrast, setContrast] = useState(() => localStorage.getItem("rx-contrast") ?? "");
  const [setup, setSetup] = useState<ProviderSetup | null | undefined>(undefined);
  // undefined until asked; false means the opening sequence is still owed.
  const [welcomed, setWelcomed] = useState<boolean | undefined>(undefined);
  const [account, setAccount] = useState<AccountState | null>(null);
  const [pack, setPack] = useState<ThemePack | null>(null);
  const [look, setLook] = useState<Look>({});
  // The metrics column is the window's, but its contents belong to the focused
  // pane, which renders into it through a portal.
  const [sideHost, setSideHost] = useState<HTMLElement | null>(null);
  // Bumped when a pane is rebound to another transcript. It rides the Pane key,
  // so the takeover remounts it: every bit of what is on screen belonged to the
  // conversation it just left.
  const [takeover, setTakeover] = useState<Record<string, number>>({});
  // Every pane's run state, so a tab can show that the conversation behind it
  // is still working. Read through a ref by the report handler, which must stay
  // stable or each frame would re-render every pane.
  const [runs, setRuns] = useState<Record<string, { run: string; live: boolean }>>({});
  const activeRef = useRef("");
  activeRef.current = active;

  const fail = useCallback((e: unknown) => setError(reason(e)), []);

  const onReport = useCallback((id: string, next: PaneReport) => {
    setRuns((prev) =>
      prev[id]?.run === next.run && prev[id]?.live === next.live ? prev : { ...prev, [id]: { run: next.run, live: next.live } },
    );
    if (id === activeRef.current) setReport(next);
  }, []);

  const reloadTree = useCallback(
    () =>
      hub
        .tree()
        .then(setTree)
        .catch(() => setTree([])),
    [hub],
  );

  // Panes and tree move together: opening a session marks its row live, closing
  // one hands the row back.
  const reloadPanes = useCallback(async () => {
    const list = await hub.runtimes().catch(() => [] as RuntimeView[]);
    setRuntimes(list);
    setActive((cur) => (list.some((rt) => rt.id === cur) ? cur : (list[0]?.id ?? "")));
    await reloadTree();
  }, [hub, reloadTree]);

  useEffect(() => {
    void reloadPanes();
  }, [reloadPanes]);

  // One port per pane, held across renders — a fresh instance would resubscribe
  // the event stream and drop the frames in between.
  const panePorts = useMemo(() => {
    const map = new Map<string, AgentPort>();
    for (const rt of runtimes) map.set(rt.id, hub.portFor(rt));
    return map;
  }, [hub, runtimes]);
  const activePort = panePorts.get(active) ?? panePorts.values().next().value ?? null;

  useEffect(() => {
    if (!activePort) return;
    let alive = true;
    activePort.providerSetup().then((v) => alive && setSetup(v)).catch(() => alive && setSetup(null));
    // A machine that cannot answer has met the app before as far as we care:
    // the sequence must never be what stands between someone and their session.
    activePort.welcomeSeen().then((v) => alive && setWelcomed(v)).catch(() => alive && setWelcomed(true));
    return () => {
      alive = false;
    };
  }, [activePort]);

  const reloadAccount = useCallback(() => {
    activePort?.account().then(setAccount).catch(() => setAccount(null));
  }, [activePort]);
  useEffect(reloadAccount, [reloadAccount]);

  const reloadThemes = useCallback(() => {
    activePort
      ?.themes()
      .then((list) => setPack(list.find((p) => p.active) ?? null))
      .catch(() => setPack(null));
  }, [activePort]);
  useEffect(reloadThemes, [reloadThemes]);

  useEffect(() => {
    activePort
      ?.appearance()
      .then((look) => {
        // The kernel's copy is the authority across machines; adopt reloads if
        // this window booted in the wrong language from a stale local cache.
        if (adoptLang(look.language)) return;
        setLook(look);
      })
      .catch(() => {});
  }, [activePort]);

  // The control moves now and the config catches up: a size or a colour that
  // waits on a round trip reads as a dead click. The kernel's answer is what
  // is kept, since it clamps what the slider sent.
  const onLook = useCallback(
    (next: Look) => {
      setLook(next);
      activePort?.saveAppearance(next).then(setLook).catch(fail);
    },
    [activePort, fail],
  );

  const running = report.run === "running";
  // A pack carries a light and a dark set, so it is repainted with the scheme
  // rather than once at load: switching the OS to dark has to move both. The
  // running flag rides along because a pack's picture recedes while a turn is
  // in flight — the transition is in CSS, this only moves the target.
  useEffect(() => {
    const mq = matchMedia("(prefers-color-scheme: dark)");
    const paint = () => {
      const scheme = theme === "auto" ? (mq.matches ? "dark" : "light") : theme;
      document.documentElement.dataset.theme = scheme;
      applyThemePack(pack, scheme as "light" | "dark", running);
      // After the pack, never before: size and type are the reader's, and a
      // palette someone else authored does not get to overrule them.
      applyLook(look, running);
    };
    paint();
    mq.addEventListener("change", paint);
    localStorage.setItem("rx-theme", theme);
    return () => mq.removeEventListener("change", paint);
  }, [theme, pack, running, look]);

  useEffect(() => {
    if (contrast) document.documentElement.dataset.contrast = contrast;
    else delete document.documentElement.dataset.contrast;
    localStorage.setItem("rx-contrast", contrast);
  }, [contrast]);

  // A pane with no session file has never been written to — the empty one every
  // window opens with. Opening a conversation takes it over instead of parking
  // a blank column next to it.
  const openPane = useCallback(
    async (req: { root?: string; sessionPath?: string }) => {
      const blank = runtimes.find((rt) => !rt.sessionPath);
      // Asking for a new session when an unused one is already open in that
      // folder: it is the pane being asked for. Rebuilding it would cost a full
      // assembly to arrive back where we started.
      if (blank && !req.sessionPath && blank.root === req.root) {
        setActive(blank.id);
        return;
      }
      // Same folder: the pane just rebinds, so nothing is torn down and a draft
      // in its composer survives. The kernel refuses a path from another
      // project's session dir, which is why the root has to match.
      if (blank && req.sessionPath && blank.root === req.root) {
        await panePorts.get(blank.id)?.resume(req.sessionPath);
        setTakeover((prev) => ({ ...prev, [blank.id]: (prev[blank.id] ?? 0) + 1 }));
        await reloadPanes();
        setActive(blank.id);
        return;
      }
      const rt = await hub.open(req);
      // Another folder needs its own runtime, so the blank one is retired
      // rather than left behind.
      if (blank && blank.id !== rt.id) await hub.close(blank.id);
      await reloadPanes();
      setActive(rt.id);
    },
    [hub, reloadPanes, runtimes, panePorts],
  );

  const closePane = useCallback(
    (id: string) => {
      void hub.close(id).then(reloadPanes).catch(fail);
    },
    [hub, reloadPanes, fail],
  );

  // Stable, or the sidebar's memo is defeated by its own handlers and a window
  // with a few hundred sessions rebuilds that whole tree on every repaint.
  const onFold = useCallback((root: string, shut: boolean) => {
    setFolded((prev) => {
      const next = new Set(prev);
      if (shut) next.add(root);
      else next.delete(root);
      return next;
    });
  }, []);
  const foldRail = useCallback(() => setRail(false), []);

  // A webview has nowhere to put a new tab, so target="_blank" opens nothing at
  // all, and letting the link navigate in place would replace the session with
  // the page. Every link leaves through the host instead.
  useEffect(() => {
    if (!activePort) return;
    const onClick = (e: MouseEvent) => {
      if (e.defaultPrevented || e.button !== 0) return;
      const link = (e.target as Element | null)?.closest?.("a[href]");
      const href = link?.getAttribute("href") ?? "";
      if (!/^https?:\/\//i.test(href)) return;
      e.preventDefault();
      void activePort.openExternal(href).catch(fail);
    };
    addEventListener("click", onClick);
    return () => removeEventListener("click", onClick);
  }, [activePort, fail]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "\\") {
        e.preventDefault();
        if (e.shiftKey) setSide((v) => !v);
        else setRail((v) => !v);
      }
      if ((e.metaKey || e.ctrlKey) && e.key === ",") {
        e.preventDefault();
        setSettings(true);
      }
      // Escape stops the turn you are looking at, not every live turn in the
      // window — the other panes are someone else's work in progress.
      if (e.key === "Escape" && running) activePort?.cancel();
    };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [activePort, running]);

  const onSettingsChanged = useCallback(() => {
    reloadAccount();
    void reloadPanes();
  }, [reloadAccount, reloadPanes]);

  // A pane's label comes from the tree row it opened, so the sidebar and the
  // tab never disagree. An unnamed session gets a number rather than a third
  // "新会话" — with several open, identical labels are the same as no labels.
  const titleFor = useCallback(
    (rt: RuntimeView, at: number) => {
      for (const ws of tree) {
        for (const session of ws.sessions) {
          if (session.runtimeId === rt.id) return session.title || session.name;
        }
      }
      return at === 0 ? "新会话" : `新会话 ${at + 1}`;
    },
    [tree],
  );

  const tabs = useMemo(
    () => runtimes.map((rt, i) => ({ rt, title: titleFor(rt, i), run: runs[rt.id]?.run ?? "idle", live: runs[rt.id]?.live ?? false })),
    [runtimes, titleFor, runs],
  );
  // The folder only earns tab space when the panes actually span more than one.
  const manyRoots = useMemo(() => new Set(runtimes.map((rt) => rt.root)).size > 1, [runtimes]);

  const closePanes = useCallback(
    (ids: string[]) => {
      void (async () => {
        for (const id of ids) await hub.close(id).catch(fail);
        await reloadPanes();
      })();
    },
    [hub, reloadPanes, fail],
  );

  // One rename for both surfaces: the tab renames by the pane's session path,
  // the sidebar by the row's — the same file either way.
  const renameSession = useCallback(
    (path: string, title: string) => {
      if (!path) return;
      void hub.renameSession(path, title).then(reloadTree).catch(fail);
    },
    [hub, reloadTree, fail],
  );

  if (setup === undefined || welcomed === undefined) return <div className="app" data-run="idle" />;
  // The sequence and the first connection are one scene, not two screens: the
  // card rises inside it after the collapse, with the introduction still above.
  // A machine that has seen the sequence but still owes a key gets the card
  // over a still scene rather than a replay.
  if ((!welcomed || setup?.required) && activePort) {
    const card = setup?.required ? (
      <Onboarding
        port={activePort}
        setup={setup}
        onDone={() => {
          setSetup(null);
          if (!welcomed) {
            setWelcomed(true);
            void activePort.markWelcomed().catch(() => {});
          }
          void reloadPanes();
        }}
      />
    ) : undefined;
    return (
      <Welcome
        variant={setup?.required ? "full" : "short"}
        replay={!welcomed}
        onDone={() => {
          setWelcomed(true);
          void activePort.markWelcomed().catch(() => {});
        }}
      >
        {card}
      </Welcome>
    );
  }

  return (
    <div
      className="app"
      data-run={report.run}
      data-rail={rail ? "on" : "off"}
      data-side={side ? "on" : "off"}
      data-plan={report.status?.plan ? "on" : "off"}
      data-apv={report.status?.toolApprovalMode ?? "ask"}
      data-prefs={settings ? "" : undefined}
      data-tabs={runtimes.length > 1 ? "" : undefined}
    >
      <Chrome
        port={activePort}
        status={report.status}
        title={report.title}
        steer={report.steer}
        theme={theme}
        onTheme={setTheme}
        onSettings={(sec) => setSettings(sec ?? true)}
        onChanged={reloadPanes}
        account={account}
      />

      <div className="cols">
        <div className="rail">
          <Workspaces
            hub={hub}
            tree={tree}
            runtimes={runtimes}
            active={active}
            folded={folded}
            onFold={onFold}
            reload={reloadTree}
            onOpen={openPane}
            onFocus={setActive}
            onClose={closePane}
            onCollapse={foldRail}
            onRename={renameSession}
            onError={fail}
          />
        </div>

        <div className="main">
          {!rail && (
            <button className="handle handle-l" onClick={() => setRail(true)} title="展开工作区栏　⌘\" aria-label="展开工作区栏">
              ›
            </button>
          )}
          {!side && (
            <button className="handle handle-r" onClick={() => setSide(true)} title="展开度量栏　⌘⇧\" aria-label="展开度量栏">
              ‹
            </button>
          )}

          {/* One conversation on screen at a time. Side by side, two panes
              squeezed each other and a glance could not tell which composer
              belonged to which run; the ones behind keep streaming either way. */}
          {tabs.length > 1 && (
            <PaneTabs
              tabs={tabs}
              active={active}
              showRoot={manyRoots}
              onFocus={setActive}
              onClose={closePanes}
              onRename={(rt, title) => renameSession(rt.sessionPath ?? "", title)}
            />
          )}

          <div className="panes">
            {runtimes.map((rt) => {
              const port = panePorts.get(rt.id);
              return port ? (
                <Pane
                  key={`${rt.id}:${takeover[rt.id] ?? 0}`}
                  rt={rt}
                  port={port}
                  title={tabs.find((t) => t.rt.id === rt.id)?.title ?? "新会话"}
                  active={rt.id === active}
                  sideHost={sideHost}
                  side={side}
                  onFocus={() => setActive(rt.id)}
                  visible={rt.id === active}
                  onReport={onReport}
                  // Panes, not just the tree: the first turn gives this pane a
                  // session path, and until /runtimes reports it the pane still
                  // looks blank — the next history row would take it over.
                  onSessionChanged={reloadPanes}
                  onFoldSide={() => setSide(false)}
                  onSettings={() => setSettings(true)}
                />
              ) : null;
            })}
            {runtimes.length === 0 && (
              <div className="panes-empty">
                <span className="mk" aria-hidden="true">
                  ⌘
                </span>
                <p className="t">{t("没有打开的会话")}</p>
                <p className="h">{t("从左边挑一个，或者在当前文件夹开一个新的")}</p>
                <button onClick={() => void openPane({ root: tree[0]?.root }).catch(fail)}>{t("开一个新会话")}</button>
              </div>
            )}
          </div>

          {error && (
            <div className="errbar" role="alert">
              <span>{error}</span>
              <button onClick={() => setError("")}>知道了</button>
            </div>
          )}
        </div>

        <div className="side" ref={setSideHost} />
      </div>

      {settings && activePort && (
        <Settings
          port={activePort}
          status={report.status}
          theme={theme}
          onTheme={setTheme}
          contrast={contrast}
          look={look}
          onLook={onLook}
          onContrast={setContrast}
          onClose={() => setSettings(false)}
          onChanged={onSettingsChanged}
          reloadThemes={reloadThemes}
          at={typeof settings === "string" ? settings : undefined}
          account={account}
          reloadAccount={reloadAccount}
        />
      )}
    </div>
  );
}
