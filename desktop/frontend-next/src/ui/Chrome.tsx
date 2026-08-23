import { useEffect, useState } from "react";
import { t } from "../i18n";
import type { AccountState, AgentPort, Preset, SessionStatus, WorkspaceInfo } from "../port/port";
import { WindowControls, zoomOnTitleBar } from "./WindowControls";

const PRESETS: [Preset, string][] = [
  ["balanced", "均衡"],
  ["delivery", "交付"],
];

const THEMES = ["auto", "light", "dark"];
const THEME_LB: Record<string, string> = { auto: "跟随系统", light: "浅色", dark: "深色" };

const base = (p: string) => p.replace(/[/\\]+$/, "").split(/[/\\]/).pop() || p;
// The filename is a timestamp and a model ref — true, and useless to read. The
// title from /sessions is always present once the turn is on disk: a generated
// one when it is ready, the first message truncated until then.
const sessionName = (title?: string, p?: string) =>
  title?.trim() || (p ? base(p).replace(/\.jsonl$/, "") : t("新会话"));

interface Props {
  // Set when the focused pane is driven from another machine, naming it.
  host?: string;
  // Null while the window has no pane: the chrome still draws, and the few
  // controls that need a session simply do nothing until one is focused.
  port: AgentPort | null;
  status: SessionStatus | null;
  title?: string;
  steer: number;
  theme: string;
  onTheme: (t: string) => void;
  onSettings: (section?: string) => void;
  account: AccountState | null;
  onChanged: () => void;
}

export function Chrome({ port, status, title, steer, theme, onTheme, onSettings, onChanged, account, host }: Props) {
  const root = status?.workspaceRoot || status?.cwd || "";
  const project = root ? base(root) : "—";
  // Only for the "隔离" tag: the folder list and the switch itself moved to the
  // sidebar, where adding one and opening one are the same gesture.
  const [ws, setWs] = useState<WorkspaceInfo | null>(null);
  useEffect(() => {
    if (!port) {
      setWs(null);
      return;
    }
    port.workspaces().then(setWs).catch(() => setWs(null));
  }, [port, root]);

  return (
    <div className="chrome" onDoubleClick={zoomOnTitleBar}>
      <span className="brand" role="img" aria-label="Reasonix" />

      <div className="crumb">
        <span className="crumb-proj" title={root}>
          {project}
        </span>
        <span className="isolab" hidden={!ws?.isolated}>
          {t("隔离")}
        </span>
        {/* Same slot as the isolated tag: both answer "this workspace is not an
            ordinary folder on this machine", so they read as one language. */}
        <span className="isolab hostlab" hidden={!host}>
          {host}
        </span>
        <span className="crumbsep">/</span>
        <b title={status?.sessionPath}>{sessionName(title, status?.sessionPath)}</b>
        <span className="crumbsep">·</span>
        <span className="goal">{status?.goal || t("交待一个任务")}</span>
      </div>

      <span className="badge" hidden={steer === 0}>
        {t("插话待送达")} <b>{steer}</b>
      </span>

      <div className="r">
        <div className="themer" role="group" aria-label={t("执行设定")}>
          {PRESETS.map(([id, lb]) => (
            <button
              key={id}
              aria-pressed={status?.preset === id}
              onClick={() => void port?.setPreset(id).then(onChanged)}
            >
              {t(lb)}
            </button>
          ))}
        </div>
        {/* Identity sits where every app puts it, but signed out it stays an
            outline in the icon cluster: an entry point, not a pitch. Reasonix
            runs fine without an account and must not imply otherwise. */}
        <button
          className="thbtn acct-btn"
          data-on={account?.signedIn ? "" : undefined}
          onClick={() => onSettings("account")}
          aria-label={account?.signedIn ? t("账号：{name}", { name: account.user?.label ?? "" }) : t("登录")}
          title={account?.signedIn ? `${account.user?.label ?? ""} <${account.user?.email ?? ""}>` : t("登录（社区与崩溃跟进，不影响使用）")}
        >
          {account?.signedIn && account.user?.label ? (
            <span className="ini" aria-hidden="true">
              {[...account.user.label][0]?.toUpperCase()}
            </span>
          ) : (
            <svg viewBox="0 0 16 16" aria-hidden="true">
              <circle cx="8" cy="5.6" r="2.6" />
              <path d="M3.2 13.4a4.8 4.8 0 0 1 9.6 0" />
            </svg>
          )}
        </button>
        {/* Same class as the theme toggle on purpose: settings belongs in the
            icon cluster's weight class, not competing with the preset control. */}
        <button className="thbtn" onClick={() => onSettings()} aria-label={t("设置")} title={t("设置　⌘,")}>
          <svg viewBox="0 0 16 16" aria-hidden="true">
            <path d="M8 5.9a2.1 2.1 0 1 0 0 4.2 2.1 2.1 0 0 0 0-4.2" />
            <path d="M12.7 9.8a1 1 0 0 0 .2 1.1l.04.04a1.2 1.2 0 1 1-1.7 1.7l-.04-.04a1 1 0 0 0-1.1-.2 1 1 0 0 0-.6.9v.11a1.2 1.2 0 1 1-2.4 0v-.06a1 1 0 0 0-.65-.9 1 1 0 0 0-1.1.2l-.04.04a1.2 1.2 0 1 1-1.7-1.7l.04-.04a1 1 0 0 0 .2-1.1 1 1 0 0 0-.9-.6h-.11a1.2 1.2 0 0 1 0-2.4h.06a1 1 0 0 0 .9-.65 1 1 0 0 0-.2-1.1l-.04-.04a1.2 1.2 0 1 1 1.7-1.7l.04.04a1 1 0 0 0 1.1.2h.05a1 1 0 0 0 .6-.9v-.11a1.2 1.2 0 1 1 2.4 0v.06a1 1 0 0 0 .6.9 1 1 0 0 0 1.1-.2l.04-.04a1.2 1.2 0 1 1 1.7 1.7l-.04.04a1 1 0 0 0-.2 1.1v.05a1 1 0 0 0 .9.6h.11a1.2 1.2 0 0 1 0 2.4h-.06a1 1 0 0 0-.9.6" />
          </svg>
        </button>
        <button
          className="thbtn"
          data-th={theme}
          aria-label={t("主题")}
          title={t("主题：{name}", { name: t(THEME_LB[theme]) })}
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
        <WindowControls />
      </div>
    </div>
  );
}
