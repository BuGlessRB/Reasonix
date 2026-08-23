import { memo, useCallback, useEffect, useState } from "react";
import { t } from "../i18n";
import type { HubPort, RuntimeView, TreeWorkspace } from "../port/hub";
import { REMOTE_STEP_LABEL, REMOTE_STEPS, type RemoteHost } from "../port/remote";

// The same ceiling the local column uses. A machine worked on for months holds
// thousands of conversations, and drawing them all is what put 98k nodes in a
// sidebar — the reader is looking at the recent end either way.
const SHOWN = 30;

interface Props {
  hub: HubPort;
  hosts: RemoteHost[];
  runtimes: RuntimeView[];
  active: string;
  onOpen: (host: string, workspace?: string, sessionPath?: string) => Promise<void>;
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

function RemoteHostsView({ hub, hosts, runtimes, active, onOpen, onFocus, onError }: Props) {
  const [busy, setBusy] = useState("");
  // What each connected machine holds. Absent until asked; null while nothing
  // is open on it, which is the state the connect button belongs to.
  const [trees, setTrees] = useState<Record<string, TreeWorkspace[] | null>>({});
  // Folded rows. A host's key is its name; a workspace's carries the host, so
  // one machine collapsing never takes a folder on another with it.
  const [shut, setShut] = useState<Set<string>>(new Set());
  // Workspaces the reader asked to see in full.
  const [whole, setWhole] = useState<Set<string>>(new Set());

  // Only a machine with a pane on it has a kernel to ask. Keyed by host so a
  // second one connecting does not re-read the first.
  const live = hosts
    .filter((h) => h.status === "connected" || h.status === "degraded")
    .map((h) => h.name)
    .join("\u0000");
  const readTrees = useCallback(async () => {
    for (const host of live ? live.split("\u0000") : []) {
      try {
        const tree = await hub.remoteTree(host);
        setTrees((held) => ({ ...held, [host]: tree }));
      } catch {
        setTrees((held) => ({ ...held, [host]: null }));
      }
    }
  }, [hub, live]);

  useEffect(() => {
    void readTrees();
  }, [readTrees]);

  const open = async (host: RemoteHost, workspace?: string, sessionPath?: string) => {
    setBusy(host.name + (sessionPath ?? workspace ?? ""));
    try {
      await onOpen(host.name, workspace ?? host.workspace, sessionPath);
      await readTrees();
    } catch (e) {
      onError(e);
    } finally {
      setBusy("");
    }
  };

  const fold = (key: string) =>
    setShut((held) => {
      const next = new Set(held);
      if (!next.delete(key)) next.add(key);
      return next;
    });

  if (!hosts.length) return null;

  return (
    <div className="rmts">
      <div className="rmtcap">{t("远程")}</div>
      {hosts.map((host) => {
        // Panes on this machine, which is also what makes the connection worth
        // holding: the link goes down with the last of them.
        const panes = runtimes.filter((rt) => rt.host === host.name);
        const tree = trees[host.name];
        const folded = shut.has(host.name);
        const working = host.status === "connecting" || host.status === "reconnecting";
        const hint = note(host);
        return (
          <div className="rmt" key={host.name} data-state={host.status}>
            <div className="rmthead" title={host.target} role="treeitem" aria-expanded={!folded}>
              <button className="twist" onClick={() => fold(host.name)} aria-label={t(folded ? "展开" : "收起")}>
                {folded ? "▸" : "▾"}
              </button>
              <i className="rmtpip" aria-hidden="true" />
              <span className="rmtname">{host.name}</span>
              <span className="rmttarget">{host.target}</span>
            </div>
            {hint && !folded ? <div className="rmtnote">{hint}</div> : null}
            {working && host.step && !folded ? <Steps step={host.step} detail={host.detail} /> : null}

            {/* Connected: the machine answers for itself, so its own folders
                and conversations are what the reader picks from. Before that
                there is only the one row, which is also the connect button —
                asking someone to say "connect" and then "open" is one act. */}
            {folded ? null : tree
              ? tree.map((ws) => {
                  const key = host.name + ":" + ws.root;
                  const folded = shut.has(key);
                  return (
                    <div key={key} className="rmtws-node">
                      <div className="rmtwsrow" role="treeitem" aria-expanded={!folded}>
                        <button className="twist" onClick={() => fold(key)} aria-label={t(folded ? "展开" : "收起")}>
                          {folded ? "▸" : "▾"}
                        </button>
                        <span className="wsname" title={ws.root} dir="ltr">
                          {ws.name}
                        </span>
                        <span className="wscount">{ws.sessions.length}</span>
                      </div>
                      {!folded && (
                        <>
                          <button
                            className="rmtopen"
                            data-busy={busy === host.name + ws.root ? "" : undefined}
                            disabled={!!busy}
                            onClick={() => void open(host, ws.root)}
                          >
                            <span className="plus" aria-hidden="true">
                              <svg viewBox="0 0 16 16">
                                <path d="M8 3.7v8.6M3.7 8h8.6" />
                              </svg>
                            </span>
                            <span className="rmtws">{t("新会话")}</span>
                          </button>
                          {(whole.has(key) ? ws.sessions : ws.sessions.slice(0, SHOWN)).map((session) => {
                            // A conversation this window already drives is a
                            // pane to focus, never a second writer for one file.
                            const held = panes.find((rt) => rt.sessionPath === session.path);
                            return (
                              <div
                                key={session.path}
                                className="sessrow"
                                role="treeitem"
                                aria-selected={held?.id === active}
                                data-on={held?.id === active ? "" : undefined}
                                data-live={held ? "" : undefined}
                                data-busy={busy === host.name + session.path ? "" : undefined}
                                onClick={() => (held ? onFocus(held.id) : void open(host, ws.root, session.path))}
                              >
                                <i className="pip" />
                                <span className="sesstitle">{session.title || session.name}</span>
                                {session.turns ? <span className="sessmeta">{session.turns}</span> : null}
                              </div>
                            );
                          })}
                          {ws.sessions.length > SHOWN && !whole.has(key) && (
                            <button
                              className="rmtmore"
                              onClick={() =>
                                setWhole((held) => {
                                  const next = new Set(held);
                                  next.add(key);
                                  return next;
                                })
                              }
                            >
                              {t("还有 {n} 个 · 全部显示", { n: ws.sessions.length - SHOWN })}
                            </button>
                          )}
                        </>
                      )}
                    </div>
                  );
                })
              : panes.map((rt) => (
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

            {!tree && !folded && (
              <button
                className="rmtopen"
                data-busy={busy.startsWith(host.name) ? "" : undefined}
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
                  <span className="rmtws" dir="ltr">
                    {host.workspace}
                  </span>
                ) : (
                  <span className="rmtws">{t("新会话")}</span>
                )}
              </button>
            )}
          </div>
        );
      })}
    </div>
  );
}

export const RemoteHosts = memo(RemoteHostsView);
