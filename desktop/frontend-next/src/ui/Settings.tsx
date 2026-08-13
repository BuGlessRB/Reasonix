import { useEffect, useRef, useState } from "react";
import type { AgentPort, ApprovalMode, ModelEntry, Preset, SessionStatus } from "../port/port";
import { arrowTabs } from "./tablist";

const PRESETS: [Preset, string, string][] = [
  ["light", "轻量", "简单就直接做，只做针对性验证，高风险才叫独立复核"],
  ["balanced", "均衡", "按复杂度自适应。这是默认档"],
  ["delivery", "交付", "完整验收证据，中高风险改动强制独立复核"],
];

const APPROVALS: [ApprovalMode, string, string][] = [
  ["ask", "询问", "每次动手前问你"],
  ["auto", "自动", "低风险自己过，写操作仍然问"],
  ["dontAsk", "不再问", "这一类记住，本会话内不再问"],
  ["yolo", "全放行", "不问了。只在你完全信任这个工作区时用"],
];

const EFFORTS = ["auto", "low", "medium", "high", "xhigh", "max"];

const THEMES: [string, string][] = [
  ["auto", "跟随系统"],
  ["light", "浅色"],
  ["dark", "深色"],
];

type Section = "session" | "model" | "tools" | "appearance" | "advanced";

const NAV: [Section, string][] = [
  ["session", "会话"],
  ["model", "模型"],
  ["tools", "工具与权限"],
  ["appearance", "外观"],
  ["advanced", "高级"],
];

// Not in this build yet: every one of these needs its own surface, and shipping
// a half version would be worse than saying where it currently lives.
const ELSEWHERE = ["MCP 服务器", "插件与技能", "Hooks", "记忆", "主题包", "机器人接入", "网络与代理", "账号与更新"];

interface Props {
  port: AgentPort;
  status: SessionStatus | null;
  theme: string;
  onTheme: (t: string) => void;
  onClose: () => void;
  onChanged: () => void;
}

export function Settings({ port, status, theme, onTheme, onClose, onChanged }: Props) {
  const [at, setAt] = useState<Section>("session");
  const [models, setModels] = useState<ModelEntry[]>([]);
  const [busy, setBusy] = useState("");
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    port.models().then(setModels).catch(() => setModels([]));
    root.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [port, onClose]);

  const run = async (what: string, fn: () => Promise<void>) => {
    setBusy(what);
    try {
      await fn();
      onChanged();
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="prefs" ref={root} tabIndex={-1}>
      <div className="prefs-hd">
        <button className="back" onClick={onClose}>
          ‹ 返回工作台
        </button>
        <span className="ttl">设置</span>
        <span className="esc">esc</span>
      </div>

      <div className="prefs-now">
        <Now k="模型" v={status?.modelRef ?? "—"} />
        <Now k="执行档" v={PRESETS.find(([id]) => id === status?.preset)?.[1] ?? "—"} />
        <Now k="推理" v={status?.effort || "auto"} />
        <Now k="批准" v={APPROVALS.find(([id]) => id === status?.toolApprovalMode)?.[1] ?? "—"} />
        <Now k="计划模式" v={status?.plan ? "开" : "关"} />
      </div>

      <div className="prefs-body">
        <nav className="prefs-nav" role="tablist" aria-label="设置分类" onKeyDown={arrowTabs}>
          {NAV.map(([id, name]) => (
            <button key={id} id={`prefs-${id}`} role="tab" aria-selected={at === id} onClick={() => setAt(id)}>
              {name}
            </button>
          ))}
        </nav>

        <div className="prefs-main" role="tabpanel" aria-labelledby={`prefs-${at}`}>
          {at === "session" && (
            <>
              <Group title="执行设定" hint="管的是规划深度、验证广度、独立复核频率 —— 不是省钱档位。切档立刻生效，不重建运行时。">
                {PRESETS.map(([id, name, desc]) => (
                  <Row key={id} on={status?.preset === id} busy={busy === id} label={name} desc={desc}
                    onClick={() => run(id, () => port.setPreset(id))} />
                ))}
              </Group>
              <Group title="计划模式" hint="开着的时候拿不到写权限：这不是提示词里的约定，是没给这个能力。">
                <Row on={status?.plan === true} label="开" desc="只读加出计划，你批准后核心自己关掉它"
                  onClick={() => run("plan-on", () => port.setPlanMode(true))} />
                <Row on={status?.plan === false} label="关" desc="正常执行"
                  onClick={() => run("plan-off", () => port.setPlanMode(false))} />
              </Group>
              <Group title="这个会话在哪写">
                <div className="kv">
                  <span className="k">工作目录</span>
                  <span className="v">{status?.cwd ?? "—"}</span>
                </div>
                <p className="note">
                  路径是会话的身份，核心只在构造时写一次，之后没有 setter。换目录请开新会话。
                </p>
              </Group>
            </>
          )}

          {at === "model" && (
            <>
              <Group title="模型" hint="切换会带着对话重建运行时；有活儿在跑的时候切不了。">
                {models.map((m) => (
                  <Row key={m.ref} on={m.ref === status?.modelRef} busy={busy === m.ref}
                    label={m.model} desc={m.provider} onClick={() => run(m.ref, () => port.setModel(m.ref))} />
                ))}
                {models.length === 0 && <p className="note">读不到模型列表。</p>}
              </Group>
              <Group title="推理强度" hint="可选档位随 provider 能力变化，auto 表示交给 provider 默认。">
                <div className="seg" role="group" aria-label="推理强度">
                  {EFFORTS.map((e) => (
                    <button key={e} aria-pressed={(status?.effort || "auto") === e}
                      onClick={() => run(e, () => port.setEffort(e))}>
                      {e}
                    </button>
                  ))}
                </div>
              </Group>
            </>
          )}

          {at === "tools" && (
            <Group title="工具批准" hint="这是唯一挡在 agent 和你的文件之间的闸。它拦下来的时候，没有第二个入口能绕过去。">
              {APPROVALS.map(([id, name, desc]) => (
                <Row key={id} on={status?.toolApprovalMode === id} busy={busy === id} danger={id === "yolo"}
                  label={name} desc={desc} onClick={() => run(id, () => port.setApprovalMode(id))} />
              ))}
            </Group>
          )}

          {at === "appearance" && (
            <Group title="主题" hint="跟随系统时，系统切换会立刻反映；手动选过就固定住。">
              {/* 三个没有说明文字的枚举值，跟推理强度是同一种选择，就该长成同一个控件 */}
              <div className="seg" data-text role="group" aria-label="主题">
                {THEMES.map(([id, name]) => (
                  <button key={id} aria-pressed={theme === id} onClick={() => onTheme(id)}>
                    {name}
                  </button>
                ))}
              </div>
            </Group>
          )}

          {at === "advanced" && (
            <Group title="还不在这一版里" hint="每一项都需要自己的界面，做半个不如先说清楚它现在在哪。">
              <div className="chips">
                {ELSEWHERE.map((x) => (
                  <span className="chip" key={x}>{x}</span>
                ))}
              </div>
              <p className="note">这些暂时留在旧版桌面端。主干站稳之后一块一块搬过来。</p>
            </Group>
          )}
        </div>
      </div>
    </div>
  );
}

function Now({ k, v }: { k: string; v: string }) {
  return (
    <span className="kv">
      <span className="k">{k}</span>
      <span className="v">{v}</span>
    </span>
  );
}

function Group({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
  return (
    <section className="grp">
      <h2>{title}</h2>
      {hint && <p className="hint">{hint}</p>}
      <div className="grp-items">{children}</div>
    </section>
  );
}

function Row({
  on, busy, danger, label, desc, onClick,
}: {
  on?: boolean; busy?: boolean; danger?: boolean; label: string; desc?: string; onClick: () => void;
}) {
  return (
    <button className="prow" data-on={on ? "" : undefined} data-danger={danger ? "" : undefined}
      onClick={onClick} disabled={busy}>
      <span className="mark" />
      <span className="tx">
        <span className="lb">{label}</span>
        {desc && <span className="ds">{desc}</span>}
      </span>
    </button>
  );
}
