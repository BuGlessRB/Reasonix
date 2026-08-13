import { useEffect, useState } from "react";
import type { AgentPort, SessionEntry, SessionStatus } from "../port/port";

interface Props {
  port: AgentPort;
  status: SessionStatus | null;
  onFold: () => void;
  onSwitched: () => void;
}

// serve drives one session at a time, so switching is a resume on the same
// controller — not a second runtime. Parallel sessions belong to the host.
export function Sessions({ port, status, onFold, onSwitched }: Props) {
  const [list, setList] = useState<SessionEntry[]>([]);
  const [busy, setBusy] = useState("");

  const load = () => port.sessions().then(setList).catch(() => setList([]));
  useEffect(() => {
    load();
  }, [port]);

  const current = status?.sessionPath || list.find((e) => e.current)?.path;

  const pick = async (e: SessionEntry) => {
    if (e.path === current || busy) return;
    setBusy(e.path);
    try {
      await port.resume(e.path);
      await load();
      onSwitched();
    } finally {
      setBusy("");
    }
  };

  return (
    <>
      <div className="rail-hd">
        <div className="lbl">
          会话<span className="c">{list.length || 1}</span>
        </div>
        <button
          className="mkbtn"
          title="新会话"
          onClick={() => port.newSession().then(load).then(onSwitched)}
        >
          ＋
        </button>
        <button className="collapse" onClick={onFold} aria-label="收起会话栏">
          ‹
        </button>
      </div>

      <div className="scroll">
        <div role="listbox" aria-label="会话">
          {list.map((e) => {
            const on = e.path === current;
            return (
              <div
                key={e.path}
                className="ws"
                role="option"
                aria-selected={on}
                tabIndex={on ? 0 : -1}
                data-busy={busy === e.path ? "" : undefined}
                onClick={() => pick(e)}
              >
                <span className="goal">
                  <i className="pip" />
                  <span>{e.title || e.name}</span>
                </span>
                <span className="meta">
                  <span className="st">{on ? (status?.running ? "运行中" : "当前") : "已归档"}</span>
                  {e.turns ? <span className="cost">{e.turns} 轮</span> : null}
                </span>
                {on && (
                  <span className="where">
                    <i className="dot" />
                    <span className="p">{status?.cwd ?? "—"}</span>
                    <span className="tag">· 直接写</span>
                  </span>
                )}
              </div>
            );
          })}
          {current && !list.some((e) => e.path === current) && (
            <div className="ws" role="option" aria-selected tabIndex={0}>
              <span className="goal">
                <i className="pip" />
                <span>{status?.goal || "新会话"}</span>
              </span>
              <span className="meta">
                <span className="st">{status?.running ? "运行中" : "当前 · 未落盘"}</span>
              </span>
              <span className="where">
                <i className="dot" />
                <span className="p">{status?.cwd ?? "—"}</span>
                <span className="tag">· 直接写</span>
              </span>
            </div>
          )}
          {list.length === 0 && !current && <div className="ws-empty">还没有会话</div>}
        </div>
      </div>

      <div className="railfoot">
        <button className="lnk">⑂ 拉隔离副本</button>
        <button className="lnk">＋ 授权目录</button>
      </div>
    </>
  );
}
