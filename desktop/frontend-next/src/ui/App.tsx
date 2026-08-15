import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react";
import type { AccountState, AgentPort, ApprovalVerdict, Checkpoint, McpEntry, ProviderSetup, RewindScope, SessionEntry, SessionStatus, WorkspaceChanges, ThemePack } from "../port/port";
import { fromHistory, initialState, quoteAmount, reduce } from "../state/session";
import { pairCheckpoints } from "../state/checkpoints";
import { initialTraj, reduceTraj } from "../state/trajectory";
import { Chrome } from "./Chrome";
import { Transcript } from "./Transcript";
import { Trajectory } from "./Trajectory";
import { Composer } from "./Composer";
import { RunStrip } from "./RunStrip";
import { SlottedView } from "./SlottedView";
import { key as slotKey, placement } from "./slots";
import { apply as applyThemePack } from "./theme";
import { Metrics } from "./Metrics";
import { Sessions } from "./Sessions";
import { Settings } from "./Settings";
import { Onboarding } from "./Onboarding";
import { Welcome } from "./Welcome";
import { arrowTabs } from "./tablist";
import { tokensPerSecond } from "../port/tokens";

export function App({ port }: { port: AgentPort }) {
  const [s, dispatch] = useReducer(reduce, initialState);
  const [traj, trajDispatch] = useReducer(reduceTraj, initialTraj);
  const [status, setStatus] = useState<SessionStatus | null>(null);
  const [rail, setRail] = useState(true);
  const [side, setSide] = useState(true);
  const [pane, setPane] = useState<"flow" | "traj">("flow");
  const [pinned, setPinned] = useState(true);
  // false = closed, true = open at its last section, a string = open there.
  const [settings, setSettings] = useState<string | boolean>(false);
  const [theme, setTheme] = useState(() => localStorage.getItem("rx-theme") ?? "auto");
  const [setup, setSetup] = useState<ProviderSetup | null | undefined>(undefined);
  // undefined until asked; false means the opening sequence is still owed.
  const [welcomed, setWelcomed] = useState<boolean | undefined>(undefined);
  const [sessions, setSessions] = useState<SessionEntry[]>([]);
  const [account, setAccount] = useState<AccountState | null>(null);
  const [mcp, setMcp] = useState<McpEntry[]>([]);
  const [pack, setPack] = useState<ThemePack | null>(null);
  const [elapsed, setElapsed] = useState(0);
  const [tps, setTps] = useState(0);
  const [tree, setTree] = useState<WorkspaceChanges | null>(null);
  const [checkpoints, setCheckpoints] = useState<Checkpoint[]>([]);
  const flow = useRef<HTMLDivElement>(null);
  const startedAt = useRef(0);
  // Read by the 250ms tick without making it a dependency, so a delta arriving
  // between ticks does not restart the interval.
  const win = useRef(s.outWindow);
  win.current = s.outWindow;

  const reloadMcp = useCallback(() => {
    void port.mcp().then(setMcp).catch(() => setMcp([]));
  }, [port]);

  useEffect(
    () =>
      port.subscribe((ev) => {
        dispatch(ev);
        trajDispatch(ev);
        // A server finishing its handshake changes what /mcp answers, and this
        // is the only precise signal for it — the turn boundary below is the
        // fallback for changes that arrive without an event.
        if (ev.kind === "mcp_surface_ready" || ev.kind === "extension_status") reloadMcp();
      }),
    [port, reloadMcp],
  );

  useEffect(() => {
    let alive = true;
    port.providerSetup().then((v) => alive && setSetup(v)).catch(() => alive && setSetup(null));
    // A machine that cannot answer has met the app before as far as we care:
    // the sequence must never be what stands between someone and their session.
    port.welcomeSeen().then((v) => alive && setWelcomed(v)).catch(() => alive && setWelcomed(true));
    return () => {
      alive = false;
    };
  }, [port]);

  useEffect(() => {
    let alive = true;
    port.trajectory().then((evs) => alive && evs.forEach((e) => trajDispatch(e))).catch(() => {});
    port.checkpoints().then((cps) => alive && setCheckpoints(cps)).catch(() => {});
    Promise.all([port.history(), port.status()]).then(([msgs, st]) => {
      if (!alive) return;
      setStatus(st);
      const restored = fromHistory(msgs);
      dispatch({
        kind: "__restore",
        items: restored.items,
        plan: restored.plan,
        hit: st.cacheHit,
        miss: st.cacheMiss,
        cost: quoteAmount(st.sessionCostQuote),
      } as never);
    });
    return () => {
      alive = false;
    };
  }, [port]);

  const refreshStatus = useCallback(() => {
    port.status().then(setStatus).catch(() => {});
  }, [port]);

  // Stable so the settings pane's own reload effect does not re-run every render.
  const reloadAccount = useCallback(() => {
    port.account().then(setAccount).catch(() => setAccount(null));
  }, [port]);

  const onSettingsChanged = useCallback(() => {
    refreshStatus();
    reloadMcp();
    reloadAccount();
  }, [refreshStatus, reloadMcp, reloadAccount]);

  const fail = useCallback((e: unknown) => {
    dispatch({ kind: "__error", text: e instanceof Error ? e.message : String(e) } as never);
  }, []);

  // Resolves once the new list is in: the delete animation has to hold the
  // collapsed row until then, or it springs back open for a round trip.
  const reloadSessions = useCallback(
    () => port.sessions().then(setSessions).catch(() => setSessions([])),
    [port],
  );

  // Everything on screen belongs to one session; when the kernel moves to
  // another one — a switch, a new session, a whole new workspace — all of it
  // has to be re-read rather than patched.
  const reloadSession = useCallback(() => {
    refreshStatus();
    reloadSessions();
    trajDispatch({ kind: "__clear" } as never);
    port.trajectory().then((evs) => evs.forEach((e) => trajDispatch(e))).catch(() => {});
    port.checkpoints().then(setCheckpoints).catch(() => setCheckpoints([]));
    port.history().then((msgs) => {
      const r = fromHistory(msgs);
      dispatch({ kind: "__restore", items: r.items, plan: r.plan, hit: 0, miss: 0 } as never);
    });
  }, [port, refreshStatus, reloadSessions]);

  // A rewind rewrites the transcript and the files under it, so the whole
  // session is re-read rather than patched — the same treatment a session
  // switch gets, for the same reason.
  const onPrepareRewind = useCallback(
    (turn: number, scope: RewindScope) => port.prepareRewind(turn, scope),
    [port],
  );
  const onCommitRewind = useCallback(
    async (planId: string) => {
      const result = await port.commitRewind(planId);
      reloadSession();
      return result;
    },
    [port, reloadSession],
  );
  const onUndoRewind = useCallback(
    (transactionId: string) => port.undoRewind(transactionId).then(reloadSession),
    [port, reloadSession],
  );

  // Keyed by item id, so a streamed frame does not change any row's own value
  // and the transcript's memo still holds.
  const paired = useMemo(() => pairCheckpoints(s.items, checkpoints), [s.items, checkpoints]);

  // The shell deliberately mints no session file at launch, so the list starts
  // empty and the first turn is what creates one — and its title only exists
  // once that turn is on disk. Re-read when the session changes or a run ends.
  // An MCP server connects lazily and fails at first use, so a turn boundary is
  // also when its status can have changed — no timer of its own needed.
  useEffect(() => {
    reloadSessions();
    reloadMcp();
    // A finished turn is exactly when the kernel has one more checkpoint.
    port.checkpoints().then(setCheckpoints).catch(() => {});
    port.changes().then(setTree).catch(() => setTree(null));
  }, [reloadSessions, reloadMcp, port, status?.sessionPath, s.running]);

  useEffect(reloadAccount, [reloadAccount]);

  // /status is the only source for background jobs and for settings the run does
  // not echo, so a live turn has to re-read it rather than infer from events.
  useEffect(() => {
    if (!s.running) {
      startedAt.current = 0;
      setTps(0);
      return;
    }
    if (!startedAt.current) startedAt.current = Date.now();
    const t = setInterval(() => {
      setElapsed((Date.now() - startedAt.current) / 1000);
      setTps(tokensPerSecond(win.current, Date.now()));
      refreshStatus();
    }, 250);
    return () => clearInterval(t);
  }, [s.running, refreshStatus]);

  // A pack carries a light and a dark set, so it is repainted with the scheme
  // rather than once at load: switching the OS to dark has to move both. The
  // running flag rides along because a pack's picture recedes while a turn is
  // in flight — the transition is in CSS, this only moves the target.
  useEffect(() => {
    const mq = matchMedia("(prefers-color-scheme: dark)");
    const paint = () => {
      const scheme = theme === "auto" ? (mq.matches ? "dark" : "light") : theme;
      document.documentElement.dataset.theme = scheme;
      applyThemePack(pack, scheme as "light" | "dark", s.running);
    };
    paint();
    mq.addEventListener("change", paint);
    localStorage.setItem("rx-theme", theme);
    return () => mq.removeEventListener("change", paint);
  }, [theme, pack, s.running]);

  const reloadThemes = useCallback(() => {
    port
      .themes()
      .then((list) => setPack(list.find((p) => p.active) ?? null))
      .catch(() => setPack(null));
  }, [port]);
  useEffect(reloadThemes, [reloadThemes]);

  // Where the user put each extension surface. Loaded once and updated in
  // place: a move is the user's own action, so waiting for a round trip to see
  // it land would read as the control ignoring the click.
  const [slots, setSlots] = useState<Record<string, string>>({});
  useEffect(() => {
    void port.surfaceSlots().then(setSlots).catch(() => setSlots({}));
  }, [port]);
  const moveSurface = useCallback(
    async (ext: { pluginId: string; surfaceId: string }, slot: string) => {
      const id = `${ext.pluginId}:${ext.surfaceId}`;
      setSlots((prev) => {
        const next = { ...prev };
        if (slot) next[id] = slot;
        else delete next[id];
        return next;
      });
      await port.assignSurface(id, slot).catch(fail);
    },
    [port, fail],
  );
  const atComposer = s.views.filter((v) => placement(v, slots) === "composer-trailing");
  const inRail = s.views.filter((v) => placement(v, slots) !== "composer-trailing");

  // A webview has nowhere to put a new tab, so target="_blank" opens nothing at
  // all, and letting the link navigate in place would replace the session with
  // the page. Every link leaves through the host instead.
  useEffect(() => {
    const onClick = (e: MouseEvent) => {
      if (e.defaultPrevented || e.button !== 0) return;
      const link = (e.target as Element | null)?.closest?.("a[href]");
      const href = link?.getAttribute("href") ?? "";
      if (!/^https?:\/\//i.test(href)) return;
      e.preventDefault();
      void port.openExternal(href).catch(fail);
    };
    addEventListener("click", onClick);
    return () => removeEventListener("click", onClick);
  }, [port, fail]);

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
      if (e.key === "Escape" && s.running) port.cancel();
    };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [port, s.running]);

  const submit = useCallback(
    (text: string) => {
      const steering = s.running;
      dispatch({ kind: "__user", text, pending: steering } as never);
      trajDispatch({ kind: "__user", text });
      if (steering) {
        port.steer(text).catch(fail);
        return;
      }
      port.submit(text).then(refreshStatus).catch(fail);
    },
    [port, s.running, refreshStatus, fail],
  );

  // Transcript rows are memoised on their item; a callback rebuilt every render
  // would defeat that on the two cards that take one.
  const onApprove = useCallback(
    (itemId: string, id: string, v: ApprovalVerdict) => {
      dispatch({ kind: "__decided", id: itemId, verdict: v } as never);
      port.approve(id, v).then(refreshStatus).catch(fail);
    },
    [port, refreshStatus, fail],
  );

  // Taking it back is only cheap while the conversation that caused it is still
  // on screen; the card stays, marked, so the record of what happened survives.
  const onForget = useCallback(
    (itemId: string, name: string) => {
      port
        .forgetMemory(name)
        .then(() => dispatch({ kind: "__forgot", id: itemId } as never))
        .catch(fail);
    },
    [port, fail],
  );

  // An extension action reports back in its own words. Surfacing the result as
  // a notice keeps it in the transcript where the card that offered it sits,
  // and a refusal lands on the same path as any other failure.
  const onExtInvoke = useCallback(
    (name: string) => {
      port
        .invokeExtensionAction(name)
        .then((message) => {
          if (message.trim()) dispatch({ kind: "notice", level: "info", text: message });
        })
        .catch(fail);
    },
    [port, fail],
  );

  const onExtSubmit = useCallback(
    (pluginId: string, surfaceId: string, values: Record<string, unknown>) => {
      port.submitExtensionForm(pluginId, surfaceId, values).catch(fail);
    },
    [port, fail],
  );

  const onAnswer = useCallback(
    (itemId: string, id: string, answers: { questionId: string; selected: string[] }[]) => {
      dispatch({ kind: "__decided", id: itemId, answers: answers.map((a) => a.selected) } as never);
      void port.answer(id, answers).catch(fail);
    },
    [port, fail],
  );

  const toLatest = () => {
    const el = flow.current;
    if (el) el.scrollTop = el.scrollHeight;
    setPinned(true);
  };

  if (setup === undefined || welcomed === undefined) return <div className="app" data-run="idle" />;
  // The sequence plays before anything else, and runs short when there is no
  // key to ask for — an introduction with nothing after it should not linger.
  if (!welcomed) {
    return (
      <Welcome
        variant={setup?.required ? "full" : "short"}
        onDone={() => {
          setWelcomed(true);
          void port.markWelcomed().catch(() => {});
        }}
      />
    );
  }
  if (setup?.required) {
    return <Onboarding port={port} setup={setup} onDone={() => { setSetup(null); refreshStatus(); }} />;
  }

  // A turn blocked on you is not a turn in motion: the glow says "it is moving"
  // and has to go out, and the action slot goes back to send so you can talk.
  const blocked = s.doing === "等你批准" || s.doing === "等你决定";
  const run = blocked ? "halt" : s.running ? "running" : s.items.length ? (s.doing === "已完成" ? "done" : "halt") : "idle";
  const steps = s.items.filter((i) => i.t === "tool").length;
  const pendingSteer = s.items.filter((i) => i.t === "user" && i.pending).length;
  const yolo = status?.toolApprovalMode === "yolo";

  return (
    <div
      className="app"
      data-run={run}
      data-rail={rail ? "on" : "off"}
      data-side={side ? "on" : "off"}
      data-plan={status?.plan ? "on" : "off"}
      data-apv={status?.toolApprovalMode ?? "ask"}
      data-prefs={settings ? "" : undefined}
    >
      <Chrome
        port={port}
        status={status}
        title={sessions.find((e) => e.current || e.path === status?.sessionPath)?.title}
        steer={pendingSteer}
        theme={theme}
        onTheme={setTheme}
        onSettings={(sec) => setSettings(sec ?? true)}
        onChanged={refreshStatus}
        account={account}
        onWorkspace={reloadSession}
        onError={fail}
      />

      <div className="cols">
        <div className="rail">
          <Sessions
            port={port}
            status={status}
            list={sessions}
            reload={reloadSessions}
            run={run}
            cost={`${s.metrics.currency}${s.metrics.cost.toFixed(2)}`}
            onError={fail}
            onFold={() => setRail(false)}
            onSwitched={reloadSession}
          />
        </div>

        <div className="main">
          {!rail && (
            <button className="handle handle-l" onClick={() => setRail(true)} title="展开会话栏　⌘\" aria-label="展开会话栏">
              ›
            </button>
          )}
          {!side && (
            <button className="handle handle-r" onClick={() => setSide(true)} title="展开度量栏　⌘⇧\" aria-label="展开度量栏">
              ‹
            </button>
          )}

          <div className="tabs" role="tablist" onKeyDown={arrowTabs}>
            <button className="tab" role="tab" aria-selected={pane === "flow"} onClick={() => setPane("flow")}>
              活动<span className="n">{s.items.length}</span>
            </button>
            <button className="tab" role="tab" aria-selected={pane === "traj"} onClick={() => setPane("traj")}>
              轨迹<span className="n">{traj.rows.length}</span>
            </button>
          </div>

          <Transcript
            items={s.items}
            takeovers={s.takeovers}
            waiting={s.waiting}
            scroll={flow}
            hidden={pane !== "flow"}
            onPinned={setPinned}
            onSuggest={submit}
            onApprove={onApprove}
            onAnswer={onAnswer}
            onForget={onForget}
            onExtInvoke={onExtInvoke}
            onExtSubmit={onExtSubmit}
            checkpoints={paired}
            onPrepareRewind={onPrepareRewind}
            onCommitRewind={onCommitRewind}
            onUndoRewind={onUndoRewind}
          />

          <div className="scroll" id="trajScroll" data-pane="traj" hidden={pane !== "traj"}>
            <Trajectory rows={traj.rows} />
          </div>

          <div className="compose">
            <button className="jump" hidden={pinned || pane !== "flow"} onClick={toLatest}>
              ↓ 回到最新
            </button>
            <span className="glowring" aria-hidden="true">
              <i />
            </span>
            <RunStrip doing={s.doing} metrics={s.metrics} steps={steps} elapsed={elapsed} />
            {/* Views the user (or the extension) put next to the composer.
                They sit above it rather than inside it: the input box is the
                one thing an extension must never be able to crowd out. */}
            {atComposer.length > 0 && (
              <div className="slotrail">
                {atComposer.map((ext) => (
                  <SlottedView
                    key={slotKey(ext)}
                    ext={ext}
                    assigned={slots}
                    onAction={(id) => void port.invokeExtensionAction(id).catch(fail)}
                    onMove={(slot) => void moveSurface(ext, slot)}
                  />
                ))}
              </div>
            )}
            <Composer
              port={port}
              status={status}
              running={s.running && !blocked}
              onSubmit={submit}
              onChanged={refreshStatus}
              onError={fail}
            />
            {s.error && (
              <div className="errbar" role="alert">
                <span>{s.error}</span>
                <button onClick={() => dispatch({ kind: "__error", text: "" } as never)}>知道了</button>
              </div>
            )}
          </div>
        </div>

        <div className="side">
          <Metrics
            metrics={s.metrics}
            plan={s.plan}
            items={s.items}
            jobs={status?.jobs ?? []}
            mcp={mcp}
            rate={tps}
            done={!s.running}
            tree={tree}
            yolo={yolo}
            onFold={() => setSide(false)}
            onSettings={() => setSettings(true)}
            panels={s.panels}
            views={inRail}
            onMoveSurface={moveSurface}
            onExtInvoke={onExtInvoke}
          />
        </div>
      </div>

      {settings && (
        <Settings
          port={port}
          status={status}
          theme={theme}
          onTheme={setTheme}
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

