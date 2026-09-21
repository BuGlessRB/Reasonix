import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type Dispatch, type SetStateAction } from "react";
import { reason } from "../i18n/kernel";
import { t } from "../i18n";
import type { AccountState, AgentPort, Appearance as Look, ProviderSetup, ThemePack } from "../port/port";
import type { HubPort, RuntimeView, TreeWorkspace } from "../port/hub";
import { Chrome } from "./Chrome";
import { useLaunchHealth } from "./launchhealth";
import { Nav } from "./Nav";
import { AccountRow } from "./AccountRow";
import { apply as applyThemePack } from "./theme";
import { apply as applyLook } from "./look";
import { adopt as adoptLang } from "../i18n";
import { Pane, type PaneReport } from "./Pane";
import { Gutter, RAIL, widthOf } from "./Gutter";
import { folded as roomGaveUp, onFolds } from "./viewport";
import { trailing } from "./trailing";
import { RemoteAsk } from "./RemoteAsk";
import { RemoteHosts } from "./RemoteHosts";
import type { RemoteAsk as RemoteAskT, RemoteHost } from "../port/remote";
import { RailSearch } from "./railsearch";
import { Boundary } from "./Boundary";
import { SettingsUnavailable } from "./SettingsUnavailable";
import { useMachineBooks } from "./machinebooks";
import { Workspaces } from "./Workspaces";
import { Sky } from "./Sky";
import { useAddWorkspace } from "./addws";
import { PaneTabs } from "./PaneTabs";
import { Onboarding } from "./Onboarding";
import { Welcome } from "./Welcome";
import { StudioIcon } from "./StudioIcon";

// Start fetching the settings chunk with the shell instead of waiting for the
// first click. It remains a separate chunk (and keeps its failure boundary),
// but opening settings no longer produces a veil while the module catches up.
const settingsModule = import("./Settings").then(
  (module) => ({ module, error: null as unknown }),
  (error: unknown) => ({ module: null, error }),
);
const Settings = lazy(async () => {
  const loaded = await settingsModule;
  if (loaded.error) throw loaded.error;
  return { default: loaded.module!.Settings };
});

const NO_REPORT: PaneReport = {
  status: null,
  title: "",
  steer: 0,
  run: "idle",
  live: false,
  cost: "",
  contextPercent: null,
  context: null,
  mcp: [],
  wallet: "",
};
const PINNED_SESSIONS_KEY = "reasonix:pinned-sessions";
// Keep a small warm set for instant back-and-forth switching. Older settled
// panes are cheap to restore from disk and expensive to leave mounted: every
// hidden pane retains a transcript, observers and markdown tree.
const WARM_PANES = 4;

function savedPins(): Set<string> {
  try {
    const raw = JSON.parse(localStorage.getItem(PINNED_SESSIONS_KEY) ?? "[]");
    return new Set(Array.isArray(raw) ? raw.filter((x): x is string => typeof x === "string") : []);
  } catch {
    return new Set();
  }
}

// 两栏共用一条规则：窄到放不下就收起，缝和把手留在原处。宽档下的那个选择要留
// 着 —— 拖窄一下再拖回来，不该把用户自己收起或展开的决定抹掉。
function useFoldAway(name: string, set: Dispatch<SetStateAction<boolean>>, wideDefault = true) {
  const wide = useRef(wideDefault);
  useEffect(() => {
    let tight = roomGaveUp(name);
    return onFolds((f) => {
      const now = f.split(" ").includes(name);
      if (now === tight) return;
      tight = now;
      set((cur) => {
        if (!now) return wide.current;
        wide.current = cur;
        return false;
      });
    });
  }, [name, set]);
}

// App is the window around the panes, not a session itself: the workspace tree,
// the chrome, the settings sheet and the theme are the window's, while every
// conversation — its transcript, metrics and event stream — lives in the Pane
// that owns it. That split is what lets two sessions run side by side.
export function App({ hub }: { hub: HubPort }) {
  const [runtimes, setRuntimes] = useState<RuntimeView[]>([]);
  const [active, setActive] = useState("");
  const [tree, setTree] = useState<TreeWorkspace[]>([]);
  // Null until asked, and null again where this kernel refuses remote panes —
  // which is what keeps the section out of a browser rather than drawing a
  // heading over a feature that cannot work there.
  const [remotes, setRemotes] = useState<RemoteHost[] | null>(null);
  // Connects in flight. A link reports "connecting" only once the dial starts,
  // so without this the poll would look at an idle list and stand down exactly
  // when the step list is what the user is waiting on.
  const [opening, setOpening] = useState(0);
  // A connect stopped for a question. One at a time by construction: the link
  // that asked is blocked until it is answered.
  const [ask, setAsk] = useState<RemoteAskT | null>(null);
  const [folded, setFolded] = useState<Set<string>>(new Set());
  // 窄到放不下工作区栏时它是收起的，而不是消失的：栏一旦从 DOM 里拿掉，把手也
  // 跟着没了，剩下的入口只有一个没人知道的快捷键。
  const [rail, setRail] = useState(() => !roomGaveUp("rail"));
  const [railScope, setRailScope] = useState<"all" | "live" | "pinned" | "archived">("all");
  const [pinnedSessions, setPinnedSessions] = useState<Set<string>>(savedPins);
  const [showProjects, setShowProjects] = useState(false);
  // 三层所有权，只在用的地方相乘：wanted（用户的选择，rail 在 useFoldAway 的
  // 记忆里、side 还在盘上）、allowed（当前视口，useFoldAway 管）、focus（临时
  // 观看状态）。focus 绝不调 setRail/chooseSide —— 那会把临时状态写回偏好，退
  // 出后就恢复不了了。它也不存盘：重开一次窗口不该还在专注里。
  const [focus, setFocus] = useState(false);
  const [railW, setRailW] = useState(() => widthOf(RAIL));
  const [report, setReport] = useState<PaneReport>(NO_REPORT);
  const [findPulse, setFindPulse] = useState(0);
  const [error, setError] = useState("");
  // false = closed, true = open at its last section, a string = open there.
  const [settings, setSettings] = useState<string | boolean>(false);
  const [browser, setBrowser] = useState(false);
  const [theme, setTheme] = useState(() => localStorage.getItem("rx-theme") ?? "auto");
  // "" means never chosen, which is what lets the system's own contrast setting
  // decide. Any explicit pick wins over it from then on.
  const [contrast, setContrast] = useState(() => localStorage.getItem("rx-contrast") ?? "");
  // 空串是「跟随语言」：中文界面本来就该比西文粗一档，样式表按 :lang 给默认。
  const [weight, setWeight] = useState(() => localStorage.getItem("rx-weight") ?? "");
  const [setup, setSetup] = useState<ProviderSetup | null | undefined>(undefined);
  // undefined until asked; false means the opening sequence is still owed.
  const [welcomed, setWelcomed] = useState<boolean | undefined>(undefined);
  const [account, setAccount] = useState<AccountState | null>(null);
  const [accountUnread, setAccountUnread] = useState("");
  const [pack, setPack] = useState<ThemePack | null>(null);
  const [look, setLook] = useState<Look>({});
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
  const reportsRef = useRef<Record<string, PaneReport>>({});
  const runsRef = useRef(runs);
  runsRef.current = runs;

  const fail = useCallback((e: unknown) => setError(reason(e)), []);
  // Asked at the moment a confirmation opens, never subscribed to: runs moves
  // on every usage round, and handing the sidebar that would rebuild a tree of
  // a few hundred sessions each frame — which is what its memo is there for.
  const liveIds = useCallback((ids: string[]) => ids.filter((id) => runsRef.current[id]?.live), []);

  const onReport = useCallback((id: string, next: PaneReport) => {
    setRuns((prev) =>
      prev[id]?.run === next.run && prev[id]?.live === next.live ? prev : { ...prev, [id]: { run: next.run, live: next.live } },
    );
    // Every pane's last report, kept in a ref so a background pane's usage
    // round does not re-render the window — and so switching tabs has
    // something to read. Rendering from state here is the same figure at the
    // cost of a window render per background beat.
    reportsRef.current[id] = next;
    if (id === activeRef.current) setReport(next);
  }, []);

  // The window's title, run pill and status belong to the pane in front. They
  // were only ever written when a pane reported, so switching to an idle pane
  // left the title naming the conversation that had last spoken — a pane that
  // has nothing to say never says it again. Freshness follows the switch here
  // rather than waiting for the new pane to happen to report.
  useEffect(() => {
    setReport(reportsRef.current[active] ?? NO_REPORT);
  }, [active]);

  const reloadTree = useCallback(
    () =>
      hub
        .tree()
        .then(setTree)
        .catch(() => setTree([])),
    [hub],
  );

  // Held beside this machine's tree, and re-read on the same beat: see
  // useMachineBooks for why that beat is one rather than two.
  const { trees: remoteTrees, reload: reloadRemoteTrees } = useMachineBooks(hub, remotes);

  // Panes and tree move together: opening a session marks its row live, closing
  // one hands the row back.
  const reloadPanes = useCallback(async () => {
    const list = await hub.runtimes().catch(() => [] as RuntimeView[]);
    setRuntimes(list);
    setActive((cur) => (list.some((rt) => rt.id === cur) ? cur : (list[0]?.id ?? "")));
    await reloadTree();
    void reloadRemoteTrees();
  }, [hub, reloadTree, reloadRemoteTrees]);

  useEffect(() => {
    void reloadPanes();
  }, [reloadPanes]);

  const reloadRemotes = useCallback(
    () =>
      hub
        .remoteHosts()
        .then(setRemotes)
        .catch(() => setRemotes(null)),
    [hub],
  );

  useEffect(() => {
    void reloadRemotes();
  }, [reloadRemotes]);

  // One timer per answer, not one that runs regardless: a link mid-connect
  // changes by the second, a settled one only when something breaks, and three
  // idle hosts should cost nothing at all.
  useEffect(() => {
    if (!remotes?.length) return;
    const working = opening > 0 || remotes.some((h) => h.status === "connecting" || h.status === "reconnecting");
    if (!working && remotes.every((h) => h.status === "idle")) return;
    const timer = setTimeout(() => void reloadRemotes(), working ? 400 : 5000);
    return () => clearTimeout(timer);
  }, [remotes, opening, reloadRemotes]);

  useEffect(() => hub.onRemoteAsk(setAsk), [hub]);

  const answerRemote = useCallback(
    (id: string, ok: boolean, text: string) => {
      setAsk(null);
      hub.answerRemote(id, ok, text);
    },
    [hub],
  );

  const openRemotePane = useCallback(
    async (host: string, workspace?: string, sessionPath?: string) => {
      setOpening((n) => n + 1);
      try {
        const view = await hub.openRemote({ host, workspace, sessionPath });
        await reloadPanes();
        setActive(view.id);
      } finally {
        setOpening((n) => n - 1);
        void reloadRemotes();
      }
    },
    [hub, reloadPanes, reloadRemotes],
  );

  // One port per pane, held across renders — a fresh instance would resubscribe
  // the event stream and drop the frames in between.
  const panePorts = useMemo(() => {
    const map = new Map<string, AgentPort>();
    for (const rt of runtimes) map.set(rt.id, hub.portFor(rt));
    return map;
  }, [hub, runtimes]);
  const activePort = panePorts.get(active) ?? panePorts.values().next().value ?? null;

  useLaunchHealth(activePort, setup, welcomed);

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

  // A kernel that does not carry sign-in refuses this with a code and a
  // sentence. Dropping it left the panel saying "checking…" for the rest of
  // the session about a question that had already been answered.
  const reloadAccount = useCallback(() => {
    activePort
      ?.account()
      .then((a) => {
        setAccount(a);
        setAccountUnread("");
      })
      .catch((e) => {
        setAccount(null);
        setAccountUnread(reason(e));
      });
  }, [activePort]);
  useEffect(reloadAccount, [reloadAccount]);

  // Appearance is the window's, not the focused pane's. A remote kernel keeps
  // its own copy, and adopting that one repaints this window because the
  // reader changed tabs — so these three read and write the local pane only.
  const lookPort = useMemo(() => {
    const local = runtimes.find((rt) => !rt.host);
    return local ? hub.portFor(local) : null;
  }, [hub, runtimes]);

  const reloadThemes = useCallback(() => {
    lookPort
      ?.themes()
      .then((list) => setPack(list.find((p) => p.active) ?? null))
      .catch(() => setPack(null));
  }, [lookPort]);
  useEffect(reloadThemes, [reloadThemes]);

  useEffect(() => {
    lookPort
      ?.appearance()
      .then((look) => {
        // The kernel's copy is the authority across machines; adopt reloads if
        // this window booted in the wrong language from a stale local cache.
        if (adoptLang(look.language)) return;
        setLook(look);
      })
      .catch(() => {});
  }, [lookPort]);

  // The control moves now and the config catches up: a size or a colour that
  // waits on a round trip reads as a dead click. The kernel's answer is still
  // what is kept, since it clamps what the slider sent.
  const saveLook = useMemo(
    () => (lookPort ? trailing((next: Look) => lookPort.saveAppearance(next), setLook, fail) : null),
    [lookPort, fail],
  );
  const onLook = useCallback(
    (next: Look) => {
      setLook(next);
      saveLook?.(next);
    },
    [saveLook],
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
      // The active pack is the same state shown by the appearance picker.
      // Applying `null` here made activation persist in the kernel while the
      // shell deliberately kept drawing the default palette, so a successful
      // click looked broken until forever. Keep the authored Studio layout,
      // but let the selected pack supply its documented colour/background
      // tokens immediately.
      applyThemePack(pack, scheme as "light" | "dark", running, contrast);
      // After the pack, never before: size and type are the reader's, and a
      // palette someone else authored does not get to overrule them.
      applyLook({ ...look, wallpaper: undefined }, running);
    };
    paint();
    mq.addEventListener("change", paint);
    localStorage.setItem("rx-theme", theme);
    return () => mq.removeEventListener("change", paint);
  }, [theme, pack, running, look, contrast]);

  useEffect(() => {
    if (contrast) document.documentElement.dataset.contrast = contrast;
    else delete document.documentElement.dataset.contrast;
    localStorage.setItem("rx-contrast", contrast);
    if (weight) document.documentElement.dataset.weight = weight;
    else delete document.documentElement.dataset.weight;
    localStorage.setItem("rx-weight", weight);
  }, [contrast, weight]);

  // A pane with no session file has never been written to — the empty one every
  // window opens with. Opening a conversation takes it over instead of parking
  // a blank column next to it.
  // A transcript can be thousands of pixels tall. Capturing it into a View
  // Transition made a simple sidebar click pay for a full-page texture before
  // the active id changed. Selection feedback should be immediate.
  const focusPane = useCallback((id: string) => setActive(id), []);
  // Settings is the next layer over the whole screen and had entry but no exit: it
  // simply vanished on unmount. A view transition can animate out an element that
  // is absent from the new state, so the tree need not stay mounted.
  const showPrefs = useCallback((sec?: string) => setSettings(sec ?? true), []);
  // The host book is edited in settings and read by the sidebar, and nothing
  // else would tell it a machine was added: an idle host reports no change to
  // poll for, so a folder added there stayed invisible until the next launch.
  const hidePrefs = useCallback(() => {
    setSettings(false);
    void reloadRemotes();
  }, [reloadRemotes]);

  const openPane = useCallback(
    async (req: { root?: string; sessionPath?: string }) => {
      const blank = runtimes.find((rt) => !rt.sessionPath);
      // Asking for a new session when an unused one is already open in that
      // folder: it is the pane being asked for. Rebuilding it would cost a full
      // assembly to arrive back where we started.
      if (blank && !req.sessionPath && blank.root === req.root) {
        focusPane(blank.id);
        return;
      }
      // Same folder: the pane just rebinds, so nothing is torn down and a draft
      // in its composer survives. The kernel refuses a path from another
      // project's session dir, which is why the root has to match.
      if (blank && req.sessionPath && blank.root === req.root) {
        await panePorts.get(blank.id)?.resume(req.sessionPath);
        setTakeover((prev) => ({ ...prev, [blank.id]: (prev[blank.id] ?? 0) + 1 }));
        focusPane(blank.id);
        void reloadPanes();
        return;
      }
      // One visible pane plus live background work is the product model. Keep a
      // few settled panes warm for quick backtracking, then reuse the oldest
      // idle pane in the same workspace instead of accumulating full hidden
      // transcripts until the kernel refuses another open.
      const idle = runtimes.filter((rt) => rt.id !== active && !runsRef.current[rt.id]?.live);
      const atCapacity = runtimes.length >= WARM_PANES;
      const reusable = atCapacity && req.sessionPath
        ? idle.find((rt) => rt.root === req.root && panePorts.has(rt.id))
        : undefined;
      if (reusable && req.sessionPath) {
        await panePorts.get(reusable.id)?.resume(req.sessionPath);
        setTakeover((prev) => ({ ...prev, [reusable.id]: (prev[reusable.id] ?? 0) + 1 }));
        focusPane(reusable.id);
        void reloadPanes();
        return;
      }
      // A different workspace cannot be resumed into the same runtime. Retire
      // one settled background pane before opening so the row never reaches the
      // old "nothing happens" max-pane failure.
      let retired = "";
      if (atCapacity && idle[0]) {
        retired = idle[0].id;
        await hub.close(retired);
      }
      const rt = await hub.open(req);
      // Another folder needs its own runtime, so the blank one is retired
      // rather than left behind.
      if (blank && blank.id !== rt.id) await hub.close(blank.id);
      setRuntimes((prev) => [...prev.filter((pane) => pane.id !== rt.id && pane.id !== blank?.id && pane.id !== retired), rt]);
      focusPane(rt.id);
      void reloadPanes();
    },
    [hub, reloadPanes, runtimes, panePorts, focusPane, active],
  );

  // Awaitable because deleting a conversation has to close its pane first and
  // then wait: the kernel refuses to erase a transcript its runtime still holds,
  // so firing the close off and deleting in the same breath races the teardown.
  // Batched because the reload behind it walks every session on disk, and a
  // folder's worth of panes must not pay for that once each.
  const closePanes = useCallback(
    async (ids: string[]) => {
      for (const id of ids) await hub.close(id);
      await reloadPanes();
    },
    [hub, reloadPanes],
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

  const adder = useAddWorkspace(hub, reloadTree, fail);
  // 每个窗口都有一个根 —— 没选过项目时那是它碰巧启动的地方。两者读起来一样，
  // 于是「从哪加项目」这句问题永远问不出口；只有内核说的 remembered 分得开。
  const [claimed, setClaimed] = useState(() => localStorage.getItem("rx-claim") === "off");
  const needsProject = !claimed && tree.every((ws) => !ws.remembered);

  useFoldAway("rail", setRail);

  const onRailW = useCallback((w: number) => {
    setRailW(w);
    localStorage.setItem(RAIL.key, String(Math.round(w)));
  }, []);

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

  // The window's shortcuts, named by the action each one performs — the same
  // identity the control on screen carries, because they are the same thing
  // asked for two ways. Written out rather than branched so the census can read
  // the set: a chain of ifs is a set nothing can enumerate.
  const shortcuts: { chord: string; shift?: boolean; action: string; run: () => void }[] = useMemo(
    () => [
      { chord: "\\", action: "rail.toggle", run: () => setRail((v) => !v) },
      { chord: ",", action: "chrome.settings", run: showPrefs },
      { chord: "f", action: "transcript.find", run: () => setFindPulse((n) => n + 1) },
    ],
    [showPrefs],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey) {
        const hit = shortcuts.find((s) => s.chord === e.key && !!s.shift === e.shiftKey);
        if (hit) {
          e.preventDefault();
          hit.run();
        }
      }
      // Escape stops the turn you are looking at, not every live turn in the
      // window — the other panes are someone else's work in progress. A pane
      // over that turn owns the press first: closing it is not stopping it.
      // Anything transient above this — a menu, a picker, the overflow bubble —
      // takes the press in the capture phase and stops it there (see
      // useDismiss), so a press that reaches this listener is one nothing else
      // wanted. Leaving focus is last: stopping a turn is the more urgent of
      // the two, and doing both on one press would be neither.
      // Escape is two actions on one key, the way send and stop share one
      // button: session.stop while a turn is live, chrome.focus otherwise.
      if (e.key === "Escape" && browser) {
        setBrowser(false);
      } else if (e.key === "Escape" && !settings) {
        if (running) activePort?.cancel();
        else setFocus(false);
      }
    };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [activePort, browser, running, settings, shortcuts]);

  // A setting changed in the pane is a fact about the session behind it, and
  // the pane is what holds that fact. Without a nudge it keeps polling only
  // while a turn runs — so approving mode, preset and model all changed on disk
  // while the screen went on showing what they were when it opened.
  const [settingsPulse, setSettingsPulse] = useState(0);
  const onSettingsChanged = useCallback(() => {
    reloadAccount();
    setSettingsPulse((n) => n + 1);
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
      return at === 0 ? t("新会话") : t("新会话 {n}", { n: at + 1 });
    },
    [tree],
  );

  const tabs = useMemo(
    () => runtimes.map((rt, i) => ({ rt, title: titleFor(rt, i), run: runs[rt.id]?.run ?? "idle", live: runs[rt.id]?.live ?? false })),
    [runtimes, titleFor, runs],
  );
  // The folder only earns tab space when the panes actually span more than one.
  const manyRoots = useMemo(() => new Set(runtimes.map((rt) => rt.root)).size > 1, [runtimes]);
  const activeRuntime = runtimes.find((rt) => rt.id === active);
  const activeWorkspace = tree.find((ws) => ws.root === activeRuntime?.root) ?? tree[0];
  // The folder a new session opens in is the one the switcher above names —
  // the window's, not whichever project happens to sit first in the tree.
  const newSessionRoot = activeWorkspace?.root;
  const sessionCount = tree.reduce((n, ws) => n + ws.sessions.filter((session) => !session.archived).length, 0);
  const archivedCount = tree.reduce((n, ws) => n + ws.sessions.filter((session) => session.archived).length, 0);
  const liveCount = liveIds(runtimes.map((rt) => rt.id)).length;

  // The tab strip has nowhere to await: closing is the end of the gesture there,
  // so a refusal has to land in the error bar rather than in a caller.
  const dropPanes = useCallback((ids: string[]) => void closePanes(ids).catch(fail), [closePanes, fail]);

  // One rename for both surfaces: the tab renames by the pane's session path,
  // the sidebar by the row's — the same file either way.
  const renameSession = useCallback(
    (path: string, title: string) => {
      if (!path) return;
      void hub.renameSession(path, title).then(reloadTree).catch(fail);
    },
    [hub, reloadTree, fail],
  );

  const archiveSession = useCallback(
    async (path: string, archived: boolean, runtimeId?: string) => {
      if (runtimeId) await closePanes([runtimeId]);
      await hub.archiveSession(path, archived);
      if (archived) {
        setPinnedSessions((current) => {
          if (!current.has(path)) return current;
          const next = new Set(current);
          next.delete(path);
          localStorage.setItem(PINNED_SESSIONS_KEY, JSON.stringify([...next]));
          return next;
        });
      }
      await reloadTree();
    },
    [closePanes, hub, reloadTree],
  );

  const togglePinnedSession = useCallback((path: string) => {
    setPinnedSessions((current) => {
      const next = new Set(current);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      localStorage.setItem(PINNED_SESSIONS_KEY, JSON.stringify([...next]));
      return next;
    });
  }, []);

  if (setup === undefined || welcomed === undefined) return <div className="app" data-run="idle" />;
  if (setup?.required && activePort) {
    return (
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
    );
  }
  if (!welcomed && activePort) {
    return (
      <Welcome
        variant="short"
        replay
        onDone={() => {
          setWelcomed(true);
          void activePort.markWelcomed().catch(() => {});
        }}
      />
    );
  }

  return (
    <div
      className="app"
      data-run={report.run}
      data-rail={rail ? "on" : "off"}
      data-side={browser ? "on" : "off"}
      data-focus={focus ? "true" : undefined}
      data-plan={report.status?.plan ? "on" : "off"}
      data-apv={report.status?.toolApprovalMode ?? "ask"}
      data-prefs={settings ? "" : undefined}
      data-tabs={runtimes.length > 1 ? "" : undefined}
      style={{ "--rail-open": `${railW}px`, "--side-open": "min(430px, 42vw)" } as CSSProperties}
    >
      {ask && <RemoteAsk ask={ask} onAnswer={answerRemote} />}

      <Chrome
        host={runtimes.find((rt) => rt.id === active)?.host}
        port={activePort}
        status={report.status}
        title={tabs.find((tab) => tab.rt.id === active)?.title ?? report.title}
        steer={report.steer}
        onSettings={showPrefs}
        onBrowser={() => setBrowser((value) => !value)}
        browser={browser}
        account={account}
        focus={focus}
        onFocus={() => setFocus((v) => !v)}
        rail={rail}
        theme={theme}
        onRail={() => setRail((v) => !v)}
        onTheme={() => setTheme((v) => (v === "dark" ? "light" : "dark"))}
      />

      {pack?.sky && <Sky />}

      <div className="cols">
        <Nav
          at={settings === false ? null : settings === true ? "" : settings}
          onGo={showPrefs}
          onHome={hidePrefs}
        />
        {/* 收起而不是卸载：卸掉就丢了侧栏的滚动位置、Inspector 的展开、当前选
            中的面板。inert 是「看不见就够不着」那一半 —— 只做视觉隐藏的话，
            屏幕上没有的栏还能被 Tab 走进去。 */}
        <div className="rail" inert={focus}>
          <div className="railscroll">
          <div className="studio-rail-head">
            <div className="studio-brand" aria-label="Reasonix Studio">
              <span className="studio-brand-mark" aria-hidden="true"><StudioIcon name="brand" /></span>
              <span className="studio-brand-word" aria-hidden="true">
                <b>reasoni<span className="studio-brand-accent">x</span></b>
                <small>studio</small>
              </span>
              <button className="studio-collapse" data-action="chrome.rail" onClick={() => setRail(false)} aria-label={t("收起工作区栏")} title={t("收起侧栏")}><StudioIcon name="panel" /></button>
            </div>
            <button
              className="studio-new-task"
              data-action="session.new"
              onClick={() => void openPane({ root: newSessionRoot }).catch(fail)}
            >
              <span aria-hidden="true"><StudioIcon name="plus" /></span>{t("新建会话")}<kbd>Alt N</kbd>
            </button>
            <button className="studio-search" data-action="workspace.search" onClick={() => document.querySelector<HTMLInputElement>(".wsfind input")?.focus()}>
              <span aria-hidden="true"><StudioIcon name="search" /></span>{t("搜索与快捷操作")}<kbd>Ctrl K</kbd>
            </button>
            <div className="studio-section-label"><span>{t("已挂载工作区")}</span><small>{tree.length}</small><button data-action="workspace.add" onClick={() => adder.add()} aria-label={t("添加工作区")} title={t("选择本地文件夹")}><StudioIcon name="plus" /></button></div>
            {activeWorkspace && (
              <div className="studio-workspace-switcher">
                <button className="studio-current-workspace" data-action="workspace.switch" aria-haspopup="listbox" aria-expanded={showProjects} onClick={() => setShowProjects((v) => !v)} title={activeWorkspace.root}>
                  <span aria-hidden="true"><StudioIcon name="folder" /></span>
                  <span><b>{activeWorkspace.name}</b><small>{t("当前聚焦 · 已挂载 {n} 个", { n: tree.length })}</small></span>
                  <i aria-hidden="true"><StudioIcon name="chevron" /></i>
                </button>
                {showProjects && (
                  <div className="studio-workspace-pop" role="listbox" aria-label={t("已挂载工作区")}>
                    <div className="studio-workspace-pop-head">
                      <span>{t("选择聚焦工作区")}</span><small>{tree.length}</small>
                    </div>
                    <div className="studio-workspace-pop-list">
                      {tree.map((workspace) => {
                        const current = workspace.root === activeWorkspace.root;
                        return (
                          <button
                            key={workspace.root}
                            role="option"
                            data-action="workspace.switch"
                            data-value={workspace.root}
                            aria-selected={current}
                            title={workspace.root}
                            onClick={() => {
                              setShowProjects(false);
                              if (current) return;
                              const openRuntime = runtimes.find((runtime) => runtime.root === workspace.root);
                              if (openRuntime) {
                                focusPane(openRuntime.id);
                                return;
                              }
                              void openPane({ root: workspace.root, sessionPath: workspace.sessions[0]?.path }).catch(fail);
                            }}
                          >
                            <span aria-hidden="true"><StudioIcon name="folder" /></span>
                            <span><b>{workspace.name}</b><small>{t("{n} 会话", { n: workspace.sessions.length })}</small></span>
                            {current && <i aria-hidden="true"><StudioIcon name="check" /></i>}
                          </button>
                        );
                      })}
                    </div>
                    <button className="studio-workspace-pop-add" data-action="workspace.add" onClick={() => { setShowProjects(false); adder.add(); }}>
                      <StudioIcon name="plus" /><span>{t("添加工作区")}</span>
                    </button>
                  </div>
                )}
              </div>
            )}
            <div className="studio-quicknav">
              <button data-action="settings.section" data-value="storage" onClick={() => showPrefs("storage")}><span aria-hidden="true"><StudioIcon name="file" /></span><b>{t("文件")}</b></button>
              <button data-action="settings.section" data-value="ext" onClick={() => showPrefs("ext")}><span aria-hidden="true"><StudioIcon name="plug" /></span><b>{t("工具与集成")}</b><small>MCP · Skills</small></button>
              <button data-action="settings.section" data-value="remote" onClick={() => showPrefs("remote")}><span aria-hidden="true"><StudioIcon name="server" /></span><b>{t("运行环境")}</b><small>Local / SSH</small></button>
            </div>
            <div className="studio-section-label studio-session-label"><span>{t("会话")}</span></div>
            <div className="studio-session-filters" role="group" aria-label={t("会话范围")}>
              <div className="studio-session-segments">
                <button data-action="session.filter" data-value="all" aria-pressed={railScope === "all"} onClick={() => setRailScope("all")}><span>{t("全部")}</span><b>{sessionCount}</b></button>
                <button data-action="session.filter" data-value="live" aria-pressed={railScope === "live"} onClick={() => setRailScope("live")}><span>{t("进行中")}</span><b>{liveCount}</b></button>
                <button data-action="session.filter" data-value="pinned" aria-pressed={railScope === "pinned"} onClick={() => setRailScope("pinned")}><span>{t("置顶")}</span><b>{pinnedSessions.size}</b></button>
                <button data-action="session.filter" data-value="archived" aria-pressed={railScope === "archived"} onClick={() => setRailScope("archived")}><span>{t("归档")}</span><b>{archivedCount}</b></button>
              </div>
              <button className="studio-filter-search" data-action="workspace.search" onClick={() => document.querySelector<HTMLInputElement>(".wsfind input")?.focus()} aria-label={t("按名称筛选会话")}><StudioIcon name="search" /></button>
            </div>
          </div>
          <RailSearch>
          <Workspaces
            hub={hub}
            tree={tree}
            runtimes={runtimes}
            active={active}
            folded={folded}
            onFold={onFold}
            reload={reloadTree}
            onOpen={openPane}
            onFocus={focusPane}
            onClose={closePanes}
            liveIds={liveIds}
            runs={runs}
            scope={railScope}
            pinned={pinnedSessions}
            onPin={togglePinnedSession}
            onPause={(runtimeId) => panePorts.get(runtimeId)?.cancel()}
            onArchive={archiveSession}
            onRename={renameSession}
            onError={fail}
            adder={adder}
          >
            {remotes ? (
              <RemoteHosts
                hub={hub}
                hosts={remotes}
                runtimes={runtimes}
                active={active}
                onOpen={openRemotePane}
                onFocus={focusPane}
                reload={reloadRemotes}
                trees={remoteTrees}
                reloadTrees={reloadRemoteTrees}
                onError={fail}
              />
            ) : null}
          </Workspaces>
          </RailSearch>
          </div>
          <div className="railfoot">
            <button className="studio-wallet" data-action="settings.section" data-value="usage" onClick={() => showPrefs("usage")}><span aria-hidden="true"><StudioIcon name="wallet" /></span><b>{t("钱包与用量")}</b>{report.wallet && <small>{report.wallet}</small>}</button>
            <div className="studio-user-foot">
              <AccountRow account={account} unread={accountUnread} onOpen={() => showPrefs("account")} />
              <span className="studio-workspace-kind">{t(account?.signedIn ? "个人工作空间" : "本地工作空间")}</span>
              <button className="studio-settings" data-action="chrome.settings" onClick={() => showPrefs()} aria-label={t("设置")}><StudioIcon name="settings" /></button>
            </div>
          </div>
        </div>

        <div className="main">
          <Gutter
            edge="l"
            span={RAIL}
            width={railW}
            label={t("调整工作区栏宽度")}
            open={rail}
            onWidth={onRailW}
            onOpen={setRail}
          />
          {/* One conversation on screen at a time. Side by side, two panes
              squeezed each other and a glance could not tell which composer
              belonged to which run; the ones behind keep streaming either way. */}
          {tabs.length > 1 && (
            <PaneTabs
              tabs={tabs}
              active={active}
              showRoot={manyRoots}
              onFocus={focusPane}
              onClose={dropPanes}
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
                  title={tabs.find((tab) => tab.rt.id === rt.id)?.title ?? t("新会话")}
                  active={rt.id === active}
                  sideHost={null}
                  side={false}
                  onFocus={() => focusPane(rt.id)}
                  visible={rt.id === active}
                  onReport={onReport}
                  // Panes, not just the tree: the first turn gives this pane a
                  // session path, and until /runtimes reports it the pane still
                  // looks blank — the next history row would take it over.
                  onSessionChanged={reloadPanes}
                  pulse={settingsPulse}
                  findPulse={findPulse}
                  needsProject={needsProject}
                  onOpenProject={() => {
                    setRail(true);
                    adder.add();
                  }}
                  onKeepHere={() => {
                    localStorage.setItem("rx-claim", "off");
                    setClaimed(true);
                  }}
                  onSettings={(section) => section ? showPrefs(section) : showPrefs()}
                  manualBrowser={rt.id === active && browser}
                  onCloseManualBrowser={() => setBrowser(false)}
                />
              ) : null;
            })}
            {runtimes.length === 0 && (
              <div className="panes-empty">
                <span className="mk" aria-hidden="true">
                  ⌘
                </span>
                <p className="t">{t("没有打开的会话")}</p>
                <p className="h">{t("从左栏选择，或在当前文件夹新建")}</p>
                <button data-action="session.new" onClick={() => void openPane({ root: newSessionRoot }).catch(fail)}>{t("新建会话")}</button>
              </div>
            )}
          </div>

          {error && (
            <div className="errbar" role="alert">
              <span>{error}</span>
              <button onClick={() => setError("")}>{t("知道了")}</button>
            </div>
          )}
        </div>

      </div>

      {settings && activePort && (
        <Boundary fallback={<SettingsUnavailable onClose={hidePrefs} />}>
        <Suspense fallback={<div className="prefs" aria-busy="true" />}>
          <Settings
            hub={hub}
            onError={fail}
            port={activePort}
            status={report.status}
            theme={theme}
            onTheme={setTheme}
            contrast={contrast}
            weight={weight}
            onWeight={setWeight}
            look={look}
            onLook={onLook}
            onContrast={setContrast}
            onClose={hidePrefs}
            onChanged={onSettingsChanged}
            onSessionsRecovered={() => void reloadTree()}
            reloadThemes={reloadThemes}
            at={typeof settings === "string" ? settings : undefined}
            account={account}
            accountUnread={accountUnread}
            reloadAccount={reloadAccount}
            workspaceRoot={activeWorkspace?.root ?? ""}
          />
        </Suspense>
        </Boundary>
      )}
    </div>
  );
}
