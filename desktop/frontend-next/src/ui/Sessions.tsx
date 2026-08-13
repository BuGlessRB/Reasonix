import { useEffect, useState } from "react";
import type { AgentPort, SessionEntry, SessionStatus } from "../port/port";

interface Props {
  port: AgentPort;
  status: SessionStatus | null;
  run: string;
  cost: string;
  onFold: () => void;
  onSwitched: () => void;
}

const RUN_ST: Record<string, string> = { running: "运行中", halt: "等你", done: "已完成", idle: "空闲" };
const WS_STATE: Record<string, string> = { running: "running", halt: "awaiting", done: "done" };

export function Sessions({ port, status, run, cost, onFold, onSwitched }: Props) {
  const [list, setList] = useState<SessionEntry[]>([]);
  const [busy, setBusy] = useState("");
  const [confirm, setConfirm] = useState("");
  const [gone, setGone] = useState("");

  const load = () => port.sessions().then(setList).catch(() => setList([]));
  useEffect(() => {
    load();
  }, [port]);

  const current = status?.sessionPath || list.find((e) => e.current)?.path;
  const shown = current && !list.some((e) => e.path === current)
    ? [{ name: "", path: current, title: status?.goal, current: true }, ...list]
    : list;

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

  const drop = async (e: SessionEntry) => {
    if (confirm !== e.path) {
      setConfirm(e.path);
      return;
    }
    setConfirm("");
    setGone(e.path);
    try {
      await port.deleteSession(e.name);
    } finally {
      setTimeout(() => {
        setGone("");
        load();
      }, 220);
    }
  };

  return (
    <>
      <div className="rail-hd">
        <div className="lbl">
          会话<span className="c">{shown.length || 1}</span>
        </div>
        <button className="mkbtn" title="新会话" onClick={() => port.newSession().then(load).then(onSwitched)}>
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
                onClick={() => pick(e)}
              >
                <span className="goal">
                  <i className="pip" />
                  <span>{e.title || e.name || "新会话"}</span>
                </span>
                <span className="meta">
                  <span className="st">{on ? RUN_ST[run] : e.turns ? `${e.turns} 轮` : "已归档"}</span>
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
                      void drop(e);
                    }}
                  >
                    {confirm === e.path ? "删除" : "×"}
                  </button>
                )}
              </div>
            );
          })}
          {shown.length === 0 && <div className="ws-empty">还没有会话</div>}
        </div>
      </div>

      <div className="railfoot">
        <button className="lnk" disabled title="需要内核提供隔离工作区接口">
          ⑂ 拉隔离副本
        </button>
        <button className="lnk" disabled title="需要内核提供目录授权接口">
          ＋ 授权目录
        </button>
      </div>
    </>
  );
}
