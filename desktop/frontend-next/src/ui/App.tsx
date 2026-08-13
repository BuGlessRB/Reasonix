import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import type { AgentPort, ProviderSetup, SessionStatus } from "../port/port";
import { fromHistory, initialState, quoteAmount, reduce } from "../state/session";
import { Transcript } from "./Transcript";
import { Composer } from "./Composer";
import { RunStrip } from "./RunStrip";
import { Metrics } from "./Metrics";
import { Sessions } from "./Sessions";
import { Settings } from "./Settings";
import { Onboarding } from "./Onboarding";

export function App({ port }: { port: AgentPort }) {
  const [s, dispatch] = useReducer(reduce, initialState);
  const [status, setStatus] = useState<SessionStatus | null>(null);
  const [rail, setRail] = useState(true);
  const [settings, setSettings] = useState(false);
  const [theme, setTheme] = useState(() => localStorage.getItem("rx-theme") ?? "auto");
  const [setup, setSetup] = useState<ProviderSetup | null | undefined>(undefined);
  const [side, setSide] = useState(true);
  const statusTick = useRef(0);

  useEffect(() => port.subscribe(dispatch), [port]);

  useEffect(() => {
    let alive = true;
    port.providerSetup().then((v) => alive && setSetup(v)).catch(() => alive && setSetup(null));
    return () => {
      alive = false;
    };
  }, [port]);

  useEffect(() => {
    let alive = true;
    Promise.all([port.history(), port.status()]).then(([msgs, st]) => {
      if (!alive) return;
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

  useEffect(() => {
    let alive = true;
    port.status().then((v) => alive && setStatus(v));
    return () => {
      alive = false;
    };
  }, [port, statusTick.current]);

  const fail = useCallback((e: unknown) => {
    dispatch({ kind: "__error", text: e instanceof Error ? e.message : String(e) } as never);
  }, []);

  const refreshStatus = useCallback(() => {
    port.status().then(setStatus);
  }, [port]);

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
      if (e.key === "Escape" && s.running) port.cancel();
    };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [port, s.running]);

  if (setup === undefined) return <div className="app" data-run="idle" />;
  if (setup?.required) {
    return <Onboarding port={port} setup={setup} onDone={() => { setSetup(null); refreshStatus(); }} />;
  }

  const run = s.running ? "running" : s.items.length ? (s.doing === "已完成" ? "done" : "halt") : "idle";

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
      <div className="chrome">
        <div className="lights">
          <i />
          <i />
          <i />
        </div>
        <button className="gear" onClick={() => setSettings(true)} aria-label="设置" title="设置">
          <svg viewBox="0 0 16 16" aria-hidden="true">
            <path d="M8 5.6a2.4 2.4 0 1 0 0 4.8 2.4 2.4 0 0 0 0-4.8" />
            <path d="M13 9.4a1.1 1.1 0 0 0 .22 1.21l.04.04a1.33 1.33 0 1 1-1.88 1.88l-.04-.04a1.1 1.1 0 0 0-1.21-.22 1.1 1.1 0 0 0-.67 1v.11a1.33 1.33 0 1 1-2.66 0v-.06a1.1 1.1 0 0 0-.72-1 1.1 1.1 0 0 0-1.21.22l-.04.04a1.33 1.33 0 1 1-1.88-1.88l.04-.04a1.1 1.1 0 0 0 .22-1.21 1.1 1.1 0 0 0-1-.67h-.11a1.33 1.33 0 0 1 0-2.66h.06a1.1 1.1 0 0 0 1-.72 1.1 1.1 0 0 0-.22-1.21l-.04-.04a1.33 1.33 0 1 1 1.88-1.88l.04.04a1.1 1.1 0 0 0 1.21.22h.05a1.1 1.1 0 0 0 .67-1v-.11a1.33 1.33 0 1 1 2.66 0v.06a1.1 1.1 0 0 0 .67 1 1.1 1.1 0 0 0 1.21-.22l.04-.04a1.33 1.33 0 1 1 1.88 1.88l-.04.04a1.1 1.1 0 0 0-.22 1.21v.05a1.1 1.1 0 0 0 1 .67h.11a1.33 1.33 0 0 1 0 2.66h-.06a1.1 1.1 0 0 0-1 .67" />
          </svg>
        </button>
        <div className="crumb">
          <b>{status?.label ?? "—"}</b>
          <span className="sep">·</span>
          <span className="goal">{status?.goal || "交待一个任务"}</span>
        </div>
      </div>

      <div className="cols">
        <div className="rail">
          <Sessions
            port={port}
            status={status}
            onFold={() => setRail(false)}
            onSwitched={() => {
              refreshStatus();
              port.history().then((msgs) => {
                const r = fromHistory(msgs);
                dispatch({ kind: "__restore", items: r.items, plan: r.plan, hit: 0, miss: 0 } as never);
              });
            }}
          />
        </div>

        <div className="main">
          {!rail && (
            <button className="handle handle-l" onClick={() => setRail(true)} aria-label="展开会话栏">
              ›
            </button>
          )}
          {!side && (
            <button className="handle handle-r" onClick={() => setSide(true)} aria-label="展开度量栏">
              ‹
            </button>
          )}
          <Transcript
            items={s.items}
            waiting={s.waiting}
            onApprove={(id, v) => port.approve(id, v).then(refreshStatus).catch(fail)}
            onAnswer={(id, answers) => void port.answer(id, answers).catch(fail)}
          />
          <div className="compose">
            <RunStrip doing={s.doing} metrics={s.metrics} />
            <Composer
              port={port}
              status={status}
              running={s.running}
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
          <Metrics metrics={s.metrics} plan={s.plan} onFold={() => setSide(false)} />
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
