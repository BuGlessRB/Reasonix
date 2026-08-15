import { useState } from "react";
import { HttpError, type AgentPort, type SessionEntry, type SessionStatus } from "../port/port";

interface Props {
  port: AgentPort;
  status: SessionStatus | null;
  // Owned by App: the header needs the current session's title too, and one
  // fetch has to serve both.
  list: SessionEntry[];
  reload: () => Promise<void>;
  run: string;
  cost: string;
  onError: (e: unknown) => void;
  onFold: () => void;
  onSwitched: () => void;
}

// Cancel is a request, not a stop: the run loop finishes the tool it is inside
// before the gate opens. Poll the kernel rather than guess a delay, and give up
// rather than hang — a refusal the user can read beats a dead row.
async function settle(port: AgentPort) {
  for (let i = 0; i < 40; i++) {
    const st = await port.status().catch(() => null);
    if (st && !st.running) return;
    await new Promise((r) => setTimeout(r, 150));
  }
}

const RUN_ST: Record<string, string> = { running: "运行中", halt: "等你", done: "已完成", idle: "空闲" };
const WS_STATE: Record<string, string> = { running: "running", halt: "awaiting", done: "done" };

export function Sessions({ port, status, list, reload, run, cost, onError, onFold, onSwitched }: Props) {
  const [busy, setBusy] = useState("");
  const [confirm, setConfirm] = useState("");
  const [gone, setGone] = useState("");
  const [stop, setStop] = useState("");

  // The kernel decides which row is current — it compares canonical paths, and
  // the same file reaches us spelled two ways (Windows folds the slug's case, a
  // resume resolves it back). Only fall back to /status when no row claims it,
  // which is the session whose file the first turn has yet to create.
  const current = list.find((e) => e.current)?.path || status?.sessionPath;
  const shown = current && !list.some((e) => e.path === current)
    ? [{ name: "", path: current, title: status?.goal, current: true }, ...list]
    : list;

  // Only the kernel knows whether a turn owns the session — the rail's own state
  // is a display posture ("halt" also means "finished, waiting for you"), and
  // reading it as "still running" claimed a busy session that was idle. So try
  // the switch, and let a 409 be what asks the user to stop the turn first.
  const pick = async (e: SessionEntry) => {
    if (e.path === current || busy) return;
    const forcing = stop === e.path;
    setStop("");
    setBusy(e.path);
    try {
      if (forcing) {
        await port.cancel();
        await settle(port);
      }
      await port.resume(e.path);
      reload();
      onSwitched();
    } catch (err) {
      if (!forcing && err instanceof HttpError && err.status === 409) {
        setStop(e.path);
        setConfirm("");
        return;
      }
      onError(err);
    } finally {
      setBusy("");
    }
  };

  const drop = async (e: SessionEntry, row: HTMLElement | null) => {
    if (confirm !== e.path) {
      setConfirm(e.path);
      return;
    }
    setConfirm("");
    // height:auto has nothing to transition from, so the row vanished instead
    // of collapsing. Pin the measured height and force a paint at it first.
    if (row) {
      row.style.height = `${row.offsetHeight}px`;
      void row.offsetHeight;
    }
    setGone(e.path);
    try {
      // The kernel refuses to delete the session it is driving, so step off it
      // first. A fresh session becomes current and the transcript clears with
      // it, which is what deleting the open conversation has to mean.
      if (e.path === current) {
        await port.newSession();
        onSwitched();
      }
      await port.deleteSession(e.name);
    } catch (err) {
      if (row) row.style.height = "";
      setGone("");
      onError(err);
      return;
    }
    // Keep the row collapsed until the refreshed list has dropped it. Clearing
    // on the timer alone re-expands it for as long as the fetch takes.
    setTimeout(() => void reload().finally(() => setGone("")), 220);
  };

  return (
    <>
      <div className="rail-hd">
        <div className="lbl">
          会话<span className="c">{shown.length}</span>
        </div>
        <button className="mkbtn" title="新会话" onClick={() => port.newSession().then(reload).then(onSwitched)}>
          ＋
        </button>
        <button className="collapse" onClick={onFold} title="收起会话栏　⌘\" aria-label="收起会话栏">
          ‹
        </button>
      </div>

      <div className="scroll">
        <div role="listbox" aria-label="工作区">
          {shown.map((e) => {
            const on = e.path === current;
            return (
              <div
                key={e.path}
                className="ws"
                role="option"
                aria-selected={on}
                tabIndex={on ? 0 : -1}
                data-s={on ? WS_STATE[run] : undefined}
                data-busy={busy === e.path ? "" : undefined}
                data-confirm={confirm === e.path ? "" : undefined}
                data-gone={gone === e.path ? "" : undefined}
                data-stop={stop === e.path ? "" : undefined}
                onClick={() => pick(e)}
                onMouseLeave={() => {
                  if (confirm === e.path) setConfirm("");
                  if (stop === e.path) setStop("");
                }}
              >
                <span className="goal">
                  <i className="pip" />
                  <span>{e.title || e.name || "新会话"}</span>
                </span>
                <span className="meta">
                  <span className="st">
                    {stop === e.path
                      ? "那边还在跑 · 再点一次停下并切过来"
                      : on
                        ? RUN_ST[run]
                        : e.turns
                          ? `${e.turns} 轮`
                          : "空会话"}
                  </span>
                  {on && <span className="cost">{cost}</span>}
                </span>
                <span className="where">
                  <i className="dot" />
                  <span className="p">{status?.workspaceRoot || status?.cwd || "—"}</span>
                  <span className="tag">· 直接写</span>
                </span>
                {e.name && (
                  <button
                    className="wsdel"
                    title="删除这个会话"
                    aria-label="删除这个会话"
                    onClick={(ev) => {
                      ev.stopPropagation();
                      void drop(e, ev.currentTarget.closest<HTMLElement>(".ws"));
                    }}
                  >
                    {confirm === e.path ? (on ? "关闭并删除" : "删除") : "×"}
                  </button>
                )}
              </div>
            );
          })}
          {shown.length === 0 && <div className="ws-empty">还没有会话</div>}
        </div>
      </div>
    </>
  );
}
