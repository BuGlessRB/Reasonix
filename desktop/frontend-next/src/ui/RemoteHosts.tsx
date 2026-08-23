import { memo, useState } from "react";
import { t } from "../i18n";
import type { RuntimeView } from "../port/hub";
import { REMOTE_STEP_LABEL, REMOTE_STEPS, type RemoteHost } from "../port/remote";

interface Props {
  hosts: RemoteHost[];
  runtimes: RuntimeView[];
  active: string;
  onOpen: (host: string, workspace?: string) => Promise<void>;
  onFocus: (id: string) => void;
  onError: (e: unknown) => void;
}

// A cold connect installs a binary on the far side, so the wait is measured in
// tens of seconds. A spinner cannot tell "downloading 40MB" from "stuck", which
// is the whole reason the kernel reports a step at all.
function Steps({ step, detail }: { step: string; detail?: string }) {
  const at = REMOTE_STEPS.indexOf(step as (typeof REMOTE_STEPS)[number]);
  if (at < 0) {
    // A step this build has no label for is shown as itself: a list that
    // silently skips one reads as a connect that stalled.
    return (
      <ul className="rmtsteps">
        <li data-at="now">
          <span className="mk">⟳</span>
          <span className="lb">{step}</span>
          {detail ? <span className="dt">{detail}</span> : null}
        </li>
      </ul>
    );
  }
  return (
    <ul className="rmtsteps">
      {REMOTE_STEPS.filter((s) => s !== "reuse").map((s, i) => {
        const state = i < at ? "done" : i === at ? "now" : "next";
        return (
          <li key={s} data-at={state}>
            <span className="mk" aria-hidden="true">
              {state === "done" ? "✓" : state === "now" ? "⟳" : "○"}
            </span>
            <span className="lb">{t(REMOTE_STEP_LABEL[s] ?? s)}</span>
            {state === "now" && detail ? <span className="dt">{detail}</span> : null}
          </li>
        );
      })}
    </ul>
  );
}

// What the pip alone cannot say. Degraded is the one that matters: the link is
// up, so nothing looks wrong until a button does nothing.
function note(host: RemoteHost): string {
  switch (host.status) {
    case "reconnecting":
      return host.attempt ? t("断了，第 {n} 次重连", { n: host.attempt }) : t("断了，正在重连");
    case "degraded":
      return host.error || t("连上了，但有转发没挂上");
    case "stopped":
      return host.error || t("已断开");
    default:
      return "";
  }
}

function RemoteHostsView({ hosts, runtimes, active, onOpen, onFocus, onError }: Props) {
  const [busy, setBusy] = useState("");
  if (!hosts.length) return null;

  const open = async (host: RemoteHost) => {
    setBusy(host.name);
    try {
      await onOpen(host.name, host.workspace);
    } catch (e) {
      onError(e);
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="rmts">
      <div className="rmtcap">{t("远程")}</div>
      {hosts.map((host) => {
        // Panes on this machine, which is also what makes the connection worth
        // holding: the link goes down with the last of them.
        const panes = runtimes.filter((rt) => rt.host === host.name);
        const working = host.status === "connecting" || host.status === "reconnecting";
        const hint = note(host);
        return (
          <div className="rmt" key={host.name} data-state={host.status}>
            <div className="rmthead" title={host.target}>
              <i className="rmtpip" aria-hidden="true" />
              <span className="rmtname">{host.name}</span>
              <span className="rmttarget">{host.target}</span>
            </div>
            {hint ? <div className="rmtnote">{hint}</div> : null}
            {working && host.step ? <Steps step={host.step} detail={host.detail} /> : null}

            {panes.map((rt) => (
              <div
                key={rt.id}
                className="sessrow"
                role="treeitem"
                aria-selected={rt.id === active}
                data-on={rt.id === active ? "" : undefined}
                data-live=""
                onClick={() => onFocus(rt.id)}
              >
                <i className="pip" />
                <span className="sesstitle">{rt.name}</span>
              </div>
            ))}

            {/* The workspace row is also the connect button: on a machine with
                no pane there is nothing else to press, and a separate control
                would ask the user to say "connect" and then "open" for one act. */}
            <button
              className="rmtopen"
              data-busy={busy === host.name ? "" : undefined}
              disabled={!!busy}
              title={host.workspace || t("这台主机还没有设默认工作区")}
              onClick={() => void open(host)}
            >
              <span className="plus" aria-hidden="true">
                <svg viewBox="0 0 16 16">
                  <path d="M8 3.7v8.6M3.7 8h8.6" />
                </svg>
              </span>
              {host.workspace ? (
                <span className="ws" dir="ltr">
                  {host.workspace}
                </span>
              ) : (
                <span className="ws">{t("新会话")}</span>
              )}
            </button>
          </div>
        );
      })}
    </div>
  );
}

export const RemoteHosts = memo(RemoteHostsView);
