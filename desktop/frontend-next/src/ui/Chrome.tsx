import type { AgentPort, Preset, SessionStatus } from "../port/port";
import { Picker } from "./Menu";

const PRESETS: [Preset, string][] = [
  ["light", "轻量"],
  ["balanced", "均衡"],
  ["delivery", "交付"],
];

const THEMES = ["auto", "light", "dark"];
const THEME_LB: Record<string, string> = { auto: "跟随系统", light: "浅色", dark: "深色" };

const base = (p: string) => p.replace(/[/\\]+$/, "").split(/[/\\]/).pop() || p;
const sessionName = (p?: string) => (p ? base(p).replace(/\.jsonl$/, "") : "新会话");

interface Props {
  port: AgentPort;
  status: SessionStatus | null;
  steer: number;
  theme: string;
  onTheme: (t: string) => void;
  onSettings: () => void;
  onChanged: () => void;
}

export function Chrome({ port, status, steer, theme, onTheme, onSettings, onChanged }: Props) {
  const root = status?.workspaceRoot || status?.cwd || "";
  const project = root ? base(root) : "—";

  return (
    <div className="chrome">
      <span className="brand" role="img" aria-label="Reasonix" />

      <div className="crumb">
        <Picker
          className="crumb-btn"
          place="top"
          current={root}
          items={[
            { value: root, label: project, desc: root || "未知工作区", right: "当前" },
            { value: "__settings", label: "设置", plain: true, divide: true, right: "⌘," },
          ]}
          onPick={(v) => v === "__settings" && onSettings()}
          label={
            <>
              <span>{project}</span>
              <span className="cv" aria-hidden="true">
                ▾
              </span>
            </>
          }
        />
        <span className="sep">/</span>
        <b>{sessionName(status?.sessionPath)}</b>
        <span className="sep">·</span>
        <span className="goal">{status?.goal || "交待一个任务"}</span>
      </div>

      <span className="badge" hidden={steer === 0}>
        插话待送达 <b>{steer}</b>
      </span>

      <div className="r">
        <div className="themer" role="group" aria-label="执行设定">
          {PRESETS.map(([id, lb]) => (
            <button
              key={id}
              aria-pressed={status?.preset === id}
              onClick={() => void port.setPreset(id).then(onChanged)}
            >
              {lb}
            </button>
          ))}
        </div>
        <button
          className="thbtn"
          data-th={theme}
          aria-label="主题"
          title={"主题：" + THEME_LB[theme]}
          onClick={() => onTheme(THEMES[(THEMES.indexOf(theme) + 1) % THEMES.length])}
        >
          <svg viewBox="0 0 16 16" aria-hidden="true">
            <path className="t-auto" d="M8 2.4a5.6 5.6 0 1 0 0 11.2 5.6 5.6 0 0 0 0-11.2" />
            <path className="t-auto t-half" d="M8 2.4v11.2a5.6 5.6 0 0 0 0-11.2Z" />
            <path
              className="t-light"
              d="M8 5.2a2.8 2.8 0 1 0 0 5.6 2.8 2.8 0 0 0 0-5.6M8 1.6v1.5M8 12.9v1.5M2.4 8H3.9M12.1 8h1.5M4.1 4.1l1 1M10.9 10.9l1 1M11.9 4.1l-1 1M5.1 10.9l-1 1"
            />
            <path className="t-dark" d="M13 9.6A5.6 5.6 0 0 1 6.4 3a5.6 5.6 0 1 0 6.6 6.6Z" />
          </svg>
        </button>
      </div>
    </div>
  );
}
