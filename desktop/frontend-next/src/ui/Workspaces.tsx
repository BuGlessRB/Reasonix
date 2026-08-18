import { memo, useState } from "react";
import { t } from "../i18n";
import type { HubPort, RuntimeView, TreeSession, TreeWorkspace } from "../port/hub";

const parentOf = (root: string) => root.replace(/[/\\]+$/, "").split(/[/\\]/).slice(-2, -1)[0] ?? "";

interface Props {
  hub: HubPort;
  tree: TreeWorkspace[];
  runtimes: RuntimeView[];
  active: string;
  // Collapsed workspaces are the window's own state, not the kernel's, so the
  // sidebar keeps them here rather than asking for them back every reload.
  folded: Set<string>;
  onFold: (root: string, folded: boolean) => void;
  reload: () => Promise<void>;
  onOpen: (req: { root?: string; sessionPath?: string }) => Promise<void>;
  onFocus: (id: string) => void;
  onClose: (ids: string[]) => Promise<void>;
  // Which of these panes are mid-turn. A callback rather than a prop: run state
  // changes constantly and this is only ever asked at confirmation time.
  liveIds: (ids: string[]) => string[];
  onCollapse: () => void;
  onRename: (path: string, title: string) => void;
  onError: (e: unknown) => void;
}

// How many of a folder's sessions get a row before the rest are summarised.
// The list is newest-first, so this is the recent end. A machine that has been
// worked on for months holds thousands of these, and drawing them all put 98k
// nodes in the sidebar — more than the transcript at 20000 turns.
const SHOWN = 30;

function WorkspacesView({ hub, tree, runtimes, active, folded, reload, onFold, onOpen, onFocus, onClose, liveIds, onCollapse, onRename, onError }: Props) {
  const [busy, setBusy] = useState("");
  const [confirm, setConfirm] = useState("");
  const [typing, setTyping] = useState(false);
  // Renaming is a pencil, not a double-click: a single click already opens the
  // session, so a double one would open it twice on the way to the edit.
  const [editing, setEditing] = useState("");
  // Folders the reader asked to see in full.
  const [whole, setWhole] = useState<Set<string>>(new Set());
  // The kernel refuses past its own ceiling either way; this only decides when
  // the button greys out instead of failing on click.
  const maxPanes = hub.maxPanes();
  const full = runtimes.length >= maxPanes;
  // Two folders can share a name — a worktree copy carries the project's own.
  // Only then is the extra word worth the room it takes.
  const twice = new Set(tree.map((w) => w.name).filter((n, i, all) => all.indexOf(n) !== i));

  const addPath = async (path: string) => {
    if (!path.trim()) return;
    setBusy("__add");
    try {
      await hub.addWorkspace(path.trim());
      await reload();
    } catch (e) {
      onError(e);
    } finally {
      setBusy("");
    }
  };

  // The native panel is the main path; typing one is the escape hatch. A
  // browser tab never learns a real path, and a shell whose panel refuses to
  // open would otherwise leave no way in at all.
  const add = async () => {
    setBusy("__add");
    try {
      const dir = await hub.pickFolder();
      if (dir === null) {
        setTyping(true);
        return;
      }
      // "" is the user closing the panel — an answer, not a reason to ask again.
      if (!dir) return;
      await hub.addWorkspace(dir);
      await reload();
    } catch (e) {
      onError(e);
    } finally {
      setBusy("");
    }
  };

  // A row that is already open is a pane to focus, never a second runtime for
  // one transcript — the kernel refuses that, and it is what forked a recovery
  // branch on every save when two writers shared a file.
  const pick = async (ws: TreeWorkspace, session: TreeSession) => {
    if (session.runtimeId) {
      onFocus(session.runtimeId);
      return;
    }
    setBusy(session.path);
    try {
      await onOpen({ root: ws.root, sessionPath: session.path });
    } catch (e) {
      onError(e);
    } finally {
      setBusy("");
    }
  };

  const startSession = async (ws: TreeWorkspace) => {
    setBusy("new:" + ws.root);
    try {
      await onOpen({ root: ws.root });
    } catch (e) {
      onError(e);
    } finally {
      setBusy("");
    }
  };

  const panesOf = (root: string) => runtimes.filter((rt) => rt.root === root).map((rt) => rt.id);

  const dropWorkspace = async (ws: TreeWorkspace) => {
    if (confirm !== ws.root) {
      setConfirm(ws.root);
      return;
    }
    setConfirm("");
    try {
      // Closed here for the same reason dropSession does it: the kernel will not
      // pull a folder out from under a pane that is writing, and leaving that to
      // the reader only means asking which of eight tabs belong to this one.
      const open = panesOf(ws.root);
      if (open.length) await onClose(open);
      await hub.removeWorkspace(ws.root);
      await reload();
    } catch (e) {
      onError(e);
    }
  };

  const dropSession = async (session: TreeSession) => {
    if (confirm !== session.path) {
      setConfirm(session.path);
      return;
    }
    setConfirm("");
    try {
      // Waited on, not just fired: the pane's runtime holds the transcript's
      // lease until it is down, and the kernel will not erase a held one.
      if (session.runtimeId) await onClose([session.runtimeId]);
      await hub.removeSession(session.path);
      await reload();
    } catch (e) {
      onError(e);
    }
  };

  return (
    <>
      <div className="rail-hd">
        <div className="lbl">
          {t("工作区")}<span className="c">{tree.length}</span>
        </div>
        <button className="collapse" onClick={onCollapse} title={t("收起工作区栏")} aria-label={t("收起工作区栏")}>
          ‹
        </button>
      </div>

      <div className="scroll">
        <div role="tree" aria-label="工作区与会话">
          {tree.map((ws) => {
            const shut = folded.has(ws.root);
            // Only while the question is on screen: panesOf walks every runtime.
            const doomed = confirm === ws.root ? panesOf(ws.root) : [];
            const busyPanes = liveIds(doomed).length;
            return (
              <div className="wsnode" key={ws.root} data-missing={ws.missing ? "" : undefined}>
                {confirm === ws.root ? (
                  <Confirm
                    what={`从列表移除「${ws.name}」？`}
                    hint={removeHint(doomed.length, busyPanes)}
                    go={t("移除")}
                    danger={busyPanes > 0}
                    onGo={() => void dropWorkspace(ws)}
                    onCancel={() => setConfirm("")}
                  />
                ) : (
                  <div className="wsrow" role="treeitem" aria-expanded={!shut} data-open={ws.open ? "" : undefined}>
                    <button className="twist" onClick={() => onFold(ws.root, !shut)} aria-label={t(shut ? "展开" : "收起")}>
                      {shut ? "▸" : "▾"}
                    </button>
                    <span className="wsname" title={ws.root}>
                      {ws.name}
                    </span>
                    {(ws.isolated || twice.has(ws.name)) && (
                      <span className="wstag">{ws.isolated ? t("隔离") : parentOf(ws.root)}</span>
                    )}
                    <span className="wscount">{ws.sessions.length}</span>
                    <button
                      className="wsdel"
                      title={t("从列表移除（不删除任何文件）")}
                      aria-label={t("从列表移除")}
                      onClick={() => setConfirm(ws.root)}
                    >
                      ×
                    </button>
                  </div>
                )}

                {/* First inside the branch, not last: a folder with thirty
                    conversations would otherwise hide this below a scroll, and
                    it reads with the list, which is newest-first anyway. */}
                {!shut && (
                  <button
                    className="newsess"
                    data-busy={busy === "new:" + ws.root ? "" : undefined}
                    disabled={full || ws.missing}
                    title={full ? t("最多同时开 {n} 个面板，先关掉一个", { n: maxPanes }) : t("在 {name} 下开一个新会话", { name: ws.name })}
                    onClick={() => void startSession(ws)}
                  >
                    <span className="plus" aria-hidden="true">
                      ＋
                    </span>
                    {t("新会话")}
                  </button>
                )}

                {!shut &&
                  (whole.has(ws.root) ? ws.sessions : ws.sessions.slice(0, SHOWN)).map((session) => {
                    const on = session.runtimeId === active;
                    if (confirm === session.path) {
                      return (
                        <Confirm
                          key={session.path}
                          what={`删除「${session.title || session.name}」？`}
                          hint={t(session.runtimeId ? "它的面板会先关掉" : "连同它的记录一起删掉")}
                          go="删除"
                          danger
                          onGo={() => void dropSession(session)}
                          onCancel={() => setConfirm("")}
                        />
                      );
                    }
                    return (
                      <div
                        key={session.path}
                        className="sessrow"
                        role="treeitem"
                        aria-selected={on}
                        data-on={on ? "" : undefined}
                        data-live={session.runtimeId ? "" : undefined}
                        data-busy={busy === session.path ? "" : undefined}
                        onClick={() => void pick(ws, session)}
                      >
                        <i className="pip" />
                        {editing === session.path ? (
                          <input
                            className="sessedit"
                            autoFocus
                            defaultValue={session.title || session.name}
                            onClick={(ev) => ev.stopPropagation()}
                            onBlur={(ev) => {
                              setEditing("");
                              const next = ev.currentTarget.value.trim();
                              if (next && next !== (session.title || session.name)) onRename(session.path, next);
                            }}
                            onKeyDown={(ev) => {
                              if (ev.key === "Enter") ev.currentTarget.blur();
                              if (ev.key === "Escape") {
                                // Abandoning a rename is not stopping the run behind it.
                                ev.stopPropagation();
                                ev.currentTarget.value = session.title || session.name;
                                ev.currentTarget.blur();
                              }
                            }}
                          />
                        ) : (
                          <span className="sesstitle">{session.title || session.name}</span>
                        )}
                        <span className="sessmeta">{session.turns ? t("{n} 轮", { n: session.turns }) : t("空会话")}</span>
                        <button
                          className="sessedit-btn"
                          title={t("重命名")}
                          aria-label={t("重命名这个会话")}
                          onClick={(ev) => {
                            ev.stopPropagation();
                            setEditing(session.path);
                          }}
                        >
                          ✎
                        </button>
                        <button
                          className="wsdel"
                          title={t("删除这个会话")}
                          aria-label={t("删除这个会话")}
                          onClick={(ev) => {
                            ev.stopPropagation();
                            setConfirm(session.path);
                          }}
                        >
                          ×
                        </button>
                      </div>
                    );
                  })}

                {!shut && !whole.has(ws.root) && ws.sessions.length > SHOWN && (
                  <button
                    className="sessmore"
                    onClick={() => setWhole((prev) => new Set(prev).add(ws.root))}
                  >
                    {t("还有 {n} 个 · 全部显示", { n: ws.sessions.length - SHOWN })}
                  </button>
                )}
              </div>
            );
          })}
          {tree.length === 0 && <div className="ws-empty">{t("还没有文件夹 —— 从下面添加一个")}</div>}
        </div>
      </div>

      <div className="rail-ft">
        {typing ? (
          <form
            className="addpath"
            onSubmit={(ev) => {
              ev.preventDefault();
              const input = ev.currentTarget.elements.namedItem("path") as HTMLInputElement | null;
              setTyping(false);
              void addPath(input?.value ?? "");
            }}
          >
            <input name="path" autoFocus placeholder={t("文件夹的完整路径")} onBlur={() => setTyping(false)} />
          </form>
        ) : (
          <button className="addws" data-busy={busy === "__add" ? "" : undefined} onClick={() => void add()}>
            <span className="plus" aria-hidden="true">
              ＋
            </span>
            {t("打开或新建项目…")}
          </button>
        )}
      </div>
    </>
  );
}

// Panes report upward on every usage round, so the window repaints often; the
// tree it holds does not change nearly that often.
export const Workspaces = memo(WorkspacesView);

// Removing a folder closes its panes, and closing one stops what it is running.
// That price is said here rather than discovered afterwards — the kernel refuses
// the removal either way, and a refusal names no pane the reader can go find.
export function removeHint(panes: number, live: number): string {
  if (panes === 0) return t("不会删除任何文件");
  if (live === 0) return t("会先关掉 {n} 个面板；不会删除任何文件", { n: panes });
  return t("会先关掉 {n} 个面板，其中 {live} 个还在跑；不会删除任何文件", { n: panes, live });
}

// 确认不跟原来那行抢位置：把「×」换成「移除」两个字，宽度一变就把文件夹名挤扁
// 了。整行换成一条问句，取消永远在手边，误点的代价是零。
function Confirm({
  what,
  hint,
  go,
  danger,
  onGo,
  onCancel,
}: {
  what: string;
  hint?: string;
  go: string;
  danger?: boolean;
  onGo: () => void;
  onCancel: () => void;
}) {
  return (
    <div className="wsconfirm" role="alertdialog" aria-label={what}>
      <div className="wsconfirm-t">
        <span className="q">{what}</span>
        {hint && <span className="h">{hint}</span>}
      </div>
      <div className="wsconfirm-a">
        <button onClick={onCancel}>{t("取消")}</button>
        <button autoFocus data-danger={danger ? "" : undefined} onClick={onGo}>
          {go}
        </button>
      </div>
    </div>
  );
}
