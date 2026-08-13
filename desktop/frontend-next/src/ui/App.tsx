import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import type { AgentPort, ApprovalVerdict, ProviderSetup, SessionEntry, SessionStatus } from "../port/port";
import { fromHistory, initialState, quoteAmount, reduce } from "../state/session";
import { initialTraj, reduceTraj } from "../state/trajectory";
import { Chrome } from "./Chrome";
import { Transcript } from "./Transcript";
import { Trajectory } from "./Trajectory";
import { Composer } from "./Composer";
import { RunStrip } from "./RunStrip";
import { Metrics } from "./Metrics";
import { Sessions } from "./Sessions";
import { Settings } from "./Settings";
import { Onboarding } from "./Onboarding";
import { arrowTabs } from "./tablist";

export function App({ port }: { port: AgentPort }) {
  const [s, dispatch] = useReducer(reduce, initialState);
  const [traj, trajDispatch] = useReducer(reduceTraj, initialTraj);
  const [status, setStatus] = useState<SessionStatus | null>(null);
  const [rail, setRail] = useState(true);
  const [side, setSide] = useState(true);
  const [pane, setPane] = useState<"flow" | "traj">("flow");
  const [pinned, setPinned] = useState(true);
  const [settings, setSettings] = useState(false);
  const [theme, setTheme] = useState(() => localStorage.getItem("rx-theme") ?? "auto");
  const [setup, setSetup] = useState<ProviderSetup | null | undefined>(undefined);
  const [sessions, setSessions] = useState<SessionEntry[]>([]);
  const [elapsed, setElapsed] = useState(0);
  const flow = useRef<HTMLDivElement>(null);
  const startedAt = useRef(0);

  useEffect(
    () =>
      port.subscribe((ev) => {
        dispatch(ev);
        trajDispatch(ev);
      }),
    [port],
  );

  useEffect(() => {
    let alive = true;
    port.providerSetup().then((v) => alive && setSetup(v)).catch(() => alive && setSetup(null));
    return () => {
      alive = false;
    };
  }, [port]);

  useEffect(() => {
    let alive = true;
    port.trajectory().then((evs) => alive && evs.forEach((e) => trajDispatch(e))).catch(() => {});
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

  const fail = useCallback((e: unknown) => {
    dispatch({ kind: "__error", text: e instanceof Error ? e.message : String(e) } as never);
  }, []);

  const reloadSessions = useCallback(() => {
    void port.sessions().then(setSessions).catch(() => setSessions([]));
  }, [port]);

  // The shell deliberately mints no session file at launch, so the list starts
  // empty and the first turn is what creates one — and its title only exists
  // once that turn is on disk. Re-read when the session changes or a run ends.
  useEffect(() => {
    reloadSessions();
  }, [reloadSessions, status?.sessionPath, s.running]);

  // /status is the only source for background jobs and for settings the run does
  // not echo, so a live turn has to re-read it rather than infer from events.
  useEffect(() => {
    if (!s.running) {
      startedAt.current = 0;
      return;
    }
    if (!startedAt.current) startedAt.current = Date.now();
    const t = setInterval(() => {
      setElapsed((Date.now() - startedAt.current) / 1000);
      refreshStatus();
    }, 1000);
    return () => clearInterval(t);
  }, [s.running, refreshStatus]);

  useEffect(() => {
    const mq = matchMedia("(prefers-color-scheme: dark)");
    const paint = () => {
      document.documentElement.dataset.theme = theme === "auto" ? (mq.matches ? "dark" : "light") : theme;
    };
    paint();
    mq.addEventListener("change", paint);
    localStorage.setItem("rx-theme", theme);
    return () => mq.removeEventListener("change", paint);
  }, [theme]);

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

  if (setup === undefined) return <div className="app" data-run="idle" />;
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
        title={sessions.find((e) => e.path === status?.sessionPath)?.title}
        steer={pendingSteer}
        theme={theme}
        onTheme={setTheme}
        onSettings={() => setSettings(true)}
        onChanged={refreshStatus}
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
            onSwitched={() => {
              refreshStatus();
              reloadSessions();
              trajDispatch({ kind: "__clear" });
              port.trajectory().then((evs) => evs.forEach((e) => trajDispatch(e))).catch(() => {});
              port.history().then((msgs) => {
                const r = fromHistory(msgs);
                dispatch({ kind: "__restore", items: r.items, plan: r.plan, hit: 0, miss: 0 } as never);
              });
            }}
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
            waiting={s.waiting}
            scroll={flow}
            hidden={pane !== "flow"}
            onPinned={setPinned}
            onSuggest={submit}
            onApprove={onApprove}
            onAnswer={onAnswer}
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
            rate={elapsed >= 1 ? s.metrics.out / elapsed : 0}
            yolo={yolo}
            onFold={() => setSide(false)}
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
          onChanged={refreshStatus}
        />
      )}
    </div>
  );
}

