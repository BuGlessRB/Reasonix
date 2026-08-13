import { useEffect, useRef, useState } from "react";
import type { AgentPort, ProviderSetup } from "../port/port";
import { RMark } from "./RMark";

interface Props {
  port: AgentPort;
  setup: ProviderSetup;
  onDone: () => void;
}

// The kernel only blocks on one thing at first launch: a usable key. Model,
// preset and effort all have defaults and are one click away in the composer,
// so asking about them here would be ceremony, not onboarding.
export function Onboarding({ port, setup, onDone }: Props) {
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(setup.error ?? "");
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => input.current?.focus(), []);

  const save = async () => {
    const v = key.trim();
    if (!v || busy) return;
    setBusy(true);
    setErr("");
    try {
      await port.saveProviderKey(v);
      onDone();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setBusy(false);
    }
  };

  return (
    <div className="onb">
      <div className="onb-card">
        <RMark className="rmark onb-r" />
        <h1 className="onb-t">Reasonix</h1>
        <p className="onb-s">
          交待一件事，它自己往下做 —— 读代码、联网查证、派子代理、改文件，每一步都留痕。
        </p>

        <div className="onb-field">
          <label htmlFor="apikey">
            {setup.provider ?? "provider"} 的 API key
            {setup.keyEnv && <code>{setup.keyEnv}</code>}
          </label>
          <input
            id="apikey"
            ref={input}
            type="password"
            value={key}
            spellCheck={false}
            autoComplete="off"
            placeholder="sk-…"
            disabled={busy}
            onChange={(e) => setKey(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && save()}
          />
          {err && <div className="onb-err">{err}</div>}
        </div>

        <button className="btn onb-go" data-primary disabled={!key.trim() || busy} onClick={save}>
          {busy ? "正在校验…" : "开始"}
        </button>

        <div className="onb-note">
          key 存进本机配置，不上传任何第三方。模型、推理强度、执行设定都有默认值，随时能在输入框那排改。
        </div>
      </div>
    </div>
  );
}
