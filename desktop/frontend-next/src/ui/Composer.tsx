import { useEffect, useRef, useState } from "react";
import type { AgentPort, ApprovalMode, ModelEntry, SessionStatus } from "../port/port";
import { Picker } from "./Menu";

const APPROVALS: [ApprovalMode, string, string][] = [
  ["ask", "询问", "每次动手前问你。"],
  ["auto", "自动", "低风险自己过，写操作仍然问。"],
  ["dontAsk", "不再问", "这一类记住，本会话不再问。"],
  ["yolo", "全放行", "不问了。只在你完全信任这个工作区时用。"],
];

const EFFORTS = ["auto", "low", "medium", "high", "xhigh", "max"];

interface Props {
  port: AgentPort;
  status: SessionStatus | null;
  running: boolean;
  onSubmit: (text: string) => void;
  onChanged: () => void;
  onError: (e: unknown) => void;
}

export function Composer({ port, status, running, onSubmit, onChanged, onError }: Props) {
  const [text, setText] = useState("");
  const [models, setModels] = useState<ModelEntry[]>([]);
  const box = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    port.models().then(setModels).catch(() => setModels([]));
  }, [port]);

  // max-height caps it at 96px; the element still has to be told to grow.
  useEffect(() => {
    const el = box.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = el.scrollHeight + "px";
  }, [text]);

  const send = () => {
    const v = text.trim();
    if (!v) return;
    setText("");
    onSubmit(v);
  };

  const apv = status?.toolApprovalMode ?? "ask";
  const modelLb = status?.modelRef?.split("/").pop() ?? status?.label ?? "—";
  const change = (p: Promise<void>) => void p.then(onChanged).catch(onError);

  return (
    <>
      <textarea
        ref={box}
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
            change(port.setPlanMode(!status?.plan));
          }
        }}
      />
      <div className="row">
        <Picker
          className="mode"
          place="bottom"
          current={status?.modelRef}
          items={models.map((m) => ({ value: m.ref, label: m.model, desc: m.provider }))}
          onPick={(ref) => change(port.setModel(ref))}
          label={
            <>
              <span className="dot" aria-hidden="true" />
              <span>{modelLb}</span>
            </>
          }
        />
        <Picker
          className="mode plain"
          place="bottom"
          current={status?.effort || "auto"}
          items={EFFORTS.map((v) => ({ value: v, label: v }))}
          onPick={(v) => change(port.setEffort(v))}
          label={<span>强度 {status?.effort || "auto"}</span>}
        />
        <button
          className="mode tog"
          aria-pressed={status?.plan ?? false}
          onClick={() => change(port.setPlanMode(!status?.plan))}
        >
          计划
        </button>
        <Picker
          className={apv === "yolo" ? "mode plain danger" : "mode plain"}
          place="bottom"
          current={apv}
          items={APPROVALS.map(([v, lb, ds]) => ({ value: v, label: lb, desc: ds }))}
          onPick={(v) => change(port.setApprovalMode(v as ApprovalMode))}
          label={<span>批准 {APPROVALS.find(([m]) => m === apv)?.[1]}</span>}
        />
        <span className="go">
          <button
            className="btn send"
            data-primary
            onClick={() => (running ? change(port.cancel()) : send())}
          >
            <span className="ic" aria-hidden="true">
              <svg className="i-send" viewBox="0 0 16 16">
                <path d="M2.8 8h9.4M8.4 4.2 12.2 8l-3.8 3.8" />
              </svg>
              <svg className="i-stop" viewBox="0 0 16 16">
                <rect x="4.8" y="4.8" width="6.4" height="6.4" rx="1.3" />
              </svg>
            </span>
            <span>{running ? "停下" : "发送"}</span>
          </button>
        </span>
      </div>
    </>
  );
}
