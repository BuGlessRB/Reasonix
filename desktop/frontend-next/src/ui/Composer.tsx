import { useEffect, useMemo, useRef, useState } from "react";
import type { AgentPort, ApprovalMode, ModelEntry, SessionStatus, SlashEntry } from "../port/port";
import { Picker } from "./Menu";
import { SlashMenu, slashMatches, slashQuery } from "./SlashMenu";

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
  const [slash, setSlash] = useState<SlashEntry[]>([]);
  const [active, setActive] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const [switching, setSwitching] = useState(false);
  const box = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    port.models().then(setModels).catch(() => setModels([]));
    // Skills and commands are discovered at build time and only change when the
    // runtime is rebuilt, so one fetch per mount is the whole story.
    port.slash().then(setSlash).catch(() => setSlash([]));
  }, [port]);

  const query = slashQuery(text);
  const hits = useMemo(
    () => (query === null ? [] : slashMatches(slash, query)),
    [slash, query],
  );
  const open = !dismissed && hits.length > 0;
  const at = Math.min(active, hits.length - 1);

  const take = (e: SlashEntry) => {
    setText("/" + e.name + " ");
    setActive(0);
    box.current?.focus();
  };

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
  // Every one of these rebuilds the runtime kernel-side (~0.4s on a real
  // session). Without a pending state the click reads as a dead control.
  const change = (p: Promise<void>) => {
    setSwitching(true);
    void p.then(onChanged).catch(onError).finally(() => setSwitching(false));
  };

  return (
    <>
      {open && <SlashMenu items={hits} active={at} onPick={take} onHover={setActive} />}
      <textarea
        ref={box}
        rows={1}
        value={text}
        placeholder="交待一个任务，回车发送…　输入 / 调用技能"
        role="combobox"
        aria-expanded={open}
        aria-controls="slashmenu"
        aria-autocomplete="list"
        aria-activedescendant={open ? `slash-${at}` : undefined}
        onChange={(e) => {
          setText(e.target.value);
          setActive(0);
          setDismissed(false);
        }}
        onKeyDown={(e) => {
          if (open && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
            e.preventDefault();
            const step = e.key === "ArrowDown" ? 1 : hits.length - 1;
            setActive((i) => (Math.min(i, hits.length - 1) + step) % hits.length);
            return;
          }
          if (open && (e.key === "Enter" || e.key === "Tab") && !e.shiftKey) {
            e.preventDefault();
            take(hits[at]);
            return;
          }
          // Esc closes the menu and stops there: reaching the app would cancel
          // the running turn, which is not what dismissing a menu means.
          if (open && e.key === "Escape") {
            e.preventDefault();
            e.stopPropagation();
            setDismissed(true);
            return;
          }
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
      <div className="row" data-busy={switching ? "" : undefined}>
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
