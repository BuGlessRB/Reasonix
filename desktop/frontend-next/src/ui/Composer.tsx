import { useState } from "react";
import type { AgentPort, ApprovalMode, Preset, SessionStatus } from "../port/port";

const APPROVALS: [ApprovalMode, string][] = [
  ["ask", "询问"],
  ["auto", "自动"],
  ["dontAsk", "不再问"],
  ["yolo", "全放行"],
];

interface Props {
  port: AgentPort;
  status: SessionStatus | null;
  running: boolean;
  onChanged: () => void;
  onError: (e: unknown) => void;
}

export function Composer({ port, status, running, onChanged, onError }: Props) {
  const [text, setText] = useState("");

  const send = () => {
    const v = text.trim();
    if (!v) return;
    setText("");
    port.submit(v).then(onChanged).catch(onError);
  };

  const act = running ? "stop" : "send";
  const apv = status?.toolApprovalMode ?? "ask";

  return (
    <>
      <textarea
        rows={1}
        value={text}
        placeholder="交待一个任务，回车发送…"
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            send();
          }
          if (e.key === "Tab" && e.shiftKey) {
            e.preventDefault();
            port.setPlanMode(!status?.plan).then(onChanged).catch(onError);
          }
        }}
      />
      <div className="row">
        <button className="mode">
          <span className="dot" />
          {status?.modelRef?.split("/").pop() ?? status?.label ?? "—"}
        </button>
        <button className="mode plain">强度 {status?.effort || "auto"}</button>
        <button
          className="mode tog"
          aria-pressed={status?.plan ?? false}
          onClick={() => void port.setPlanMode(!status?.plan).then(onChanged).catch(onError)}
        >
          计划
        </button>
        <button
          className={apv === "yolo" ? "mode plain danger" : "mode plain"}
          onClick={() => {
            const i = APPROVALS.findIndex(([m]) => m === apv);
            void port.setApprovalMode(APPROVALS[(i + 1) % APPROVALS.length][0]).then(onChanged).catch(onError);
          }}
        >
          批准 {APPROVALS.find(([m]) => m === apv)?.[1]}
        </button>
        <span className="go">
          <button
            className="btn send"
            data-primary
            onClick={() => (act === "stop" ? void port.cancel().then(onChanged).catch(onError) : send())}
          >
            <span>{act === "stop" ? "停下" : "发送"}</span>
          </button>
        </span>
      </div>
    </>
  );
}

export const PRESETS: [Preset, string][] = [
  ["light", "轻量"],
  ["balanced", "均衡"],
  ["delivery", "交付"],
];
