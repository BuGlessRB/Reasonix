import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import { host } from "../port/host";
import type {
  AgentPort,
  BrowserTab,
  WorkspaceChange,
  WorkspaceFile,
} from "../port/port";
import { AgentBrowserPanel, ManualBrowserPanel, UNREAD_TABS } from "./BrowserPanel";
import { DiffView } from "./cards/DiffView";
import { StudioIcon } from "./StudioIcon";

// The editor and its grammars load with the first file opened, not with Studio.
const CodeEditor = lazy(() => import("./CodeEditor"));

type Surface =
  | { kind: "manual"; id: string }
  | { kind: "browser"; tab: BrowserTab }
  | { kind: "file"; path: string };
type TreeRow = {
  kind: "folder" | "file";
  path: string;
  name: string;
  depth: number;
};

function keyOf(s: Surface) {
  return `${s.kind}:${s.kind === "manual" ? s.id : s.kind === "browser" ? s.tab.target : s.path}`;
}
function labelOf(s: Surface, hosts: Record<string, string>) {
  return s.kind === "manual"
    ? hosts[s.id] || t("浏览器")
    : s.kind === "browser"
      ? s.tab.title || s.tab.url || t("空白页")
      : s.path.split("/").at(-1) || s.path;
}

/** Folders first at every level, with the tree still reading as a tree. Sorting
 *  flat paths as text interleaves `bin/` with `boot.test.exe`, because text is
 *  the only thing it compares. Segment by segment, whichever row is a directory
 *  where the two paths part wins there, and its children stay under it. */
function before(a: TreeRow, b: TreeRow): number {
  const x = a.path.split("/"), y = b.path.split("/");
  for (let i = 0; i < Math.min(x.length, y.length); i++) {
    if (x[i] === y[i]) continue;
    const dirs = [i < x.length - 1 || a.kind === "folder", i < y.length - 1 || b.kind === "folder"];
    return dirs[0] === dirs[1] ? x[i].localeCompare(y[i]) : dirs[0] ? -1 : 1;
  }
  return x.length - y.length;
}

function treeRows(
  files: string[],
  directories: string[],
  query: string,
  closed: Set<string>,
): TreeRow[] {
  const matches = query
    ? files.filter((path) =>
        path.toLocaleLowerCase().includes(query.toLocaleLowerCase()),
      )
    : files;
  const folders = new Set<string>(directories);
  for (const path of matches)
    for (let i = 1, bits = path.split("/"); i < bits.length; i++)
      folders.add(bits.slice(0, i).join("/"));
  const hidden = (path: string) =>
    [...closed].some(
      (folder) => path === folder || path.startsWith(`${folder}/`),
    );
  const rows: TreeRow[] = [...folders]
    .filter(
      (path) => ![...closed].some((parent) => path.startsWith(`${parent}/`)),
    )
    .map((path) => ({
      kind: "folder",
      path,
      name: path.split("/").at(-1)!,
      depth: path.split("/").length - 1,
    }));
  rows.push(
    ...matches
      .filter((path) => !hidden(path.split("/").slice(0, -1).join("/")))
      .map((path) => ({
        kind: "file" as const,
        path,
        name: path.split("/").at(-1)!,
        depth: path.split("/").length - 1,
      })),
  );
  return rows.sort(before);
}

export function WorkbenchPanel({
  port,
  tabs,
  manual,
  shown,
  scheme,
  changes,
  onCloseManual,
  onSurfaces,
  onExternal,
}: {
  port: AgentPort;
  tabs: BrowserTab[];
  manual: boolean;
  shown: boolean;
  scheme: "light" | "dark";
  changes: WorkspaceChange[];
  onCloseManual: () => void;
  // How many surfaces the strip holds, for the pane's own tab to count.
  onSurfaces: (n: number) => void;
  onExternal: (url: string) => void;
}) {
  const [files, setFiles] = useState<string[]>([]),
    [directories, setDirectories] = useState<string[]>([]),
    [openFiles, setOpenFiles] = useState<string[]>([]);
  // Docked, the body is too narrow for two columns, so the explorer is hidden
  // — and the workbench is always docked now, which left the files with no way
  // in at all. The toggle is that way in, at any width.
  const [showFiles, setShowFiles] = useState(false);
  const [query, setQuery] = useState(""),
    [selected, setSelected] = useState("");
  const [dismissed, setDismissed] = useState<Set<string>>(() => new Set()),
    [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const [browsers, setBrowsers] = useState<string[]>([]),
    [hosts, setHosts] = useState<Record<string, string>>({});
  const minted = useRef(0);
  const [mode, setMode] = useState<"file" | "diff">("file");
  const [file, setFile] = useState<WorkspaceFile | null>(null),
    [draft, setDraft] = useState(""),
    [diff, setDiff] = useState("");
  const [busy, setBusy] = useState(false),
    [failed, setFailed] = useState("");
  // Which editor opened it, or why none did. The host knows both and says so,
  // because a button that does nothing is indistinguishable from a broken one.
  const [editorNote, setEditorNote] = useState("");
  const openEditor = useCallback(() => {
    void port
      .openInEditor()
      .then(({ editor }) => setEditorNote(t("已在 {app} 中打开", { app: editor })))
      .catch((e) => setEditorNote(reason(e)));
  }, [port]);
  const changeKey = changes
    .map((change) => `${change.status}:${change.path}`)
    .join("\n");
  useEffect(() => {
    if (shown)
      port.workspaceFiles("", query).then(
        (r) => {
          setFiles(r.files);
          setDirectories(r.directories);
          setCollapsed(query ? new Set() : new Set(r.directories));
        },
        (e) => setFailed(reason(e)),
      );
  }, [port, shown, changeKey, query]);
  // The button in the chrome says "show me the browser", not "show me this one
  // browser". When the agent has a page, that page is the browser; the start
  // page is only for an empty column, and steps aside unused once the agent
  // opens something. Closing the panel is not closing the pages: they come
  // back with it, and only a tab's own × takes one away.
  const agentPage = tabs.find((tab) => tab.active) ?? tabs[0];
  const agentTarget = agentPage?.target ?? "";
  const agentAt = agentPage ? `${agentPage.target} ${agentPage.url}` : "";
  const visited = useRef(hosts);
  visited.current = hosts;
  const dropBlankStart = useCallback(() => setBrowsers((open) => open.filter((id) => !!visited.current[id])), []);
  const agentNow = useRef(agentTarget);
  agentNow.current = agentTarget;
  useEffect(() => {
    if (!manual) return;
    if (agentNow.current) {
      dropBlankStart();
      setSelected(`browser:${agentNow.current}`);
      return;
    }
    // A shell that draws views opens a view here too: a framed page is refused
    // by any site sending X-Frame-Options or frame-ancestors, as many do.
    if (host().drawsBrowserViews()) {
      port
        .browserOpen("about:blank", true)
        .then((tab) => setSelected(`browser:${tab.target}`))
        .catch((e) => setFailed(reason(e)));
      return;
    }
    setBrowsers((open) => (open.length ? open : ["b0"]));
    setSelected((at) => (at.startsWith("manual:") ? at : "manual:b0"));
  }, [manual, dropBlankStart, port]);
  // A page that has just appeared is the one somebody wants to see, whoever
  // opened it: the agent's popup, or a link the person followed, which opens
  // beside the agent's tab without becoming the one it acts on.
  const known = useRef<Set<string> | null>(null);
  useEffect(() => {
    if (tabs === UNREAD_TABS) return;
    const ids = tabs.map((tab) => tab.target);
    if (known.current === null) {
      known.current = new Set(ids);
      return;
    }
    const fresh = ids.filter((id) => !known.current!.has(id));
    for (const id of ids) known.current.add(id);
    if (fresh.length) {
      dropBlankStart();
      setSelected(`browser:${fresh[fresh.length - 1]}`);
    }
  }, [tabs, dropBlankStart]);
  // Where the agent is looking, followed as it moves: a new page, or the same
  // tab sent somewhere else.
  const lastAgent = useRef<string | null>(null);
  useEffect(() => {
    const before = lastAgent.current;
    lastAgent.current = agentAt;
    if (before === null || !agentAt || before === agentAt) return;
    dropBlankStart();
    setSelected(`browser:${agentTarget}`);
  }, [agentAt, agentTarget, dropBlankStart]);
  const noteHost = useCallback(
    (id: string, host: string) =>
      setHosts((v) => (v[id] === host ? v : { ...v, [id]: host })),
    [],
  );
  const surfaces = useMemo<Surface[]>(
    () =>
      [
        ...browsers.map((id) => ({ kind: "manual", id }) as const),
        ...tabs.map((tab) => ({ kind: "browser", tab }) as const),
        ...openFiles.map((path) => ({ kind: "file", path }) as const),
      ].filter((s) => !dismissed.has(keyOf(s))),
    [browsers, tabs, openFiles, dismissed],
  );
  const active =
    surfaces.find((s) => keyOf(s) === selected) ??
    surfaces.find((s) => s.kind === "browser" && s.tab.active) ??
    surfaces[0];
  const rows = useMemo(
    () => treeRows(files, directories, query, collapsed),
    [files, directories, query, collapsed],
  );
  useEffect(() => onSurfaces(surfaces.length), [onSurfaces, surfaces.length]);
  // A strip wider than the column scrolls, and the tab being shown is always
  // brought into it: a selected tab nobody can see reads as a tab that closed.
  const strip = useRef<HTMLDivElement>(null);
  const activeKey = active ? keyOf(active) : "";
  useEffect(() => {
    strip.current?.querySelector('[aria-selected="true"]')?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
  }, [activeKey, surfaces.length]);
  useEffect(() => {
    if (!active || active.kind !== "file") return;
    let live = true;
    setBusy(true);
    setFailed("");
    Promise.all([
      port.workspaceFile(active.path),
      port
        .changeDiff(active.path)
        .catch(() => ({ path: active.path, diff: "", truncated: false })),
    ])
      .then(
        ([next, patch]) => {
          if (live) {
            setFile(next);
            setDraft(next.content);
            setDiff(patch.diff);
          }
        },
        (e) => live && setFailed(reason(e)),
      )
      .finally(() => live && setBusy(false));
    return () => {
      live = false;
    };
  }, [port, active?.kind === "file" ? active.path : ""]);
  // Picking a file is asking to read it. Docked, the list and the file share one
  // column, so the list steps aside; side by side it stays where it is.
  const openFile = (path: string) => {
    setShowFiles(false);
    setOpenFiles((v) => (v.includes(path) ? v : [...v, path]));
    setDismissed((v) => {
      const n = new Set(v);
      n.delete(`file:${path}`);
      return n;
    });
    setSelected(`file:${path}`);
  };
  const toggleFolder = async (path: string) => {
    if (!collapsed.has(path)) {
      setCollapsed((v) => new Set(v).add(path));
      return;
    }
    try {
      const result = await port.workspaceFiles(path);
      setFiles((v) => [...new Set([...v, ...result.files])]);
      setDirectories((v) => [...new Set([...v, ...result.directories])]);
      setCollapsed((v) => {
        const n = new Set(v);
        n.delete(path);
        result.directories.forEach((dir) => n.add(dir));
        return n;
      });
    } catch (e) {
      setFailed(reason(e));
    }
  };
  // One browser: a tab opened here is a tab the agent can read and drive, and
  // it is drawn as a view rather than framed — which is the only way a site
  // that refuses to be framed opens at all. A shell with no views has no such
  // page to give, so there the iframe surface is still what there is.
  const addBrowser = async () => {
    if (!host().drawsBrowserViews()) {
      const id = `b${(minted.current += 1)}`;
      setBrowsers((v) => [...v, id]);
      setSelected(`manual:${id}`);
      return;
    }
    try {
      const tab = await port.browserOpen("about:blank", true);
      setSelected(`browser:${tab.target}`);
    } catch (e) {
      setFailed(reason(e));
    }
  };
  // The column folds only when its last tab goes, whatever kind that tab is: a
  // page opened with + or a file still open is a reason to keep it.
  const close = (surface: Surface) => {
    const key = keyOf(surface);
    if (surface.kind === "manual") setBrowsers((v) => v.filter((id) => id !== surface.id));
    else if (surface.kind === "file")
      setOpenFiles((v) => v.filter((p) => p !== surface.path));
    else setDismissed((v) => new Set(v).add(key));
    if (selected === key) setSelected("");
    if (surfaces.every((s) => keyOf(s) === key)) onCloseManual();
  };
  const save = async () => {
    if (!file || draft === file.content) return;
    setBusy(true);
    setFailed("");
    try {
      setFile(await port.saveWorkspaceFile({ ...file, content: draft }));
    } catch (e) {
      setFailed(reason(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="workbench" aria-label={t("工作台")}>
      <header className="workbench-tabs">
        <div
          className="workbench-tablist"
          role="tablist"
          ref={strip}
          onWheel={(e) => {
            if (e.deltaY && strip.current) strip.current.scrollLeft += e.deltaY;
          }}
        >
        {surfaces.map((surface) => (
          <div
            className="workbench-tab"
            key={keyOf(surface)}
            role="tab"
            aria-selected={surface === active}
            data-live={surface.kind === "browser" && surface.tab.active ? "" : undefined}
          >
            <button
              className="workbench-tab-pick"
              data-action="workbench.tab"
              data-target={keyOf(surface)}
              title={
                surface.kind === "browser"
                  ? `${surface.tab.active ? `${t("模型正在操作这个页面")}
` : ""}${surface.tab.url}`
                  : labelOf(surface, hosts)
              }
              onClick={() => setSelected(keyOf(surface))}
            >
              <StudioIcon name={surface.kind === "file" ? "file" : "globe"} />
              <span>{labelOf(surface, hosts)}</span>
            </button>
            <button
              className="workbench-tab-close"
              data-action="workbench.close"
              data-target={keyOf(surface)}
              aria-label={t("关闭 {name}", { name: labelOf(surface, hosts) })}
              title={t("关闭")}
              onClick={() => close(surface)}
            >
              <StudioIcon name="close" />
            </button>
          </div>
        ))}
        </div>
        <button
          className="workbench-files"
          data-action="workbench.files"
          aria-pressed={showFiles}
          aria-label={t("文件")}
          title={t("文件")}
          onClick={() => setShowFiles((on) => !on)}
        >
          <StudioIcon name="folder" />
        </button>
        <button
          className="workbench-new"
          data-action="workbench.new-browser"
          aria-label={t("新建浏览器标签")}
          title={t("新建浏览器标签")}
          onClick={() => void addBrowser()}
        >
          <StudioIcon name="plus" />
        </button>
      </header>
      <div className="workbench-body" data-files={showFiles ? "" : undefined}>
        <main className="workbench-canvas">
          {!active && (
            <div className="workbench-empty">
              <StudioIcon name="file" />
              <span>{t("从右侧选择文件，或打开浏览器")}</span>
            </div>
          )}
          {/* Every open browser stays mounted: a tab switch must not throw away
              where that page had got to. */}
          {browsers.map((id) => (
            <ManualBrowserPanel
              key={id}
              id={id}
              hidden={!(active?.kind === "manual" && active.id === id)}
              onAddress={noteHost}
              onExternal={onExternal}
              scheme={scheme}
            />
          ))}
          {active?.kind === "browser" && (
            <AgentBrowserPanel
              tabs={[active.tab]}
              shown={shown}
              showTabs={false}
            />
          )}
          {active?.kind === "file" && (
            <>
              <div className="workbench-filebar">
                <strong>{active.path}</strong>
                <div role="group">
                  <button
                    data-action="workbench.mode"
                    data-value="file"
                    aria-pressed={mode === "file"}
                    onClick={() => setMode("file")}
                  >
                    {t("文件")}
                  </button>
                  <button
                    data-action="workbench.mode"
                    data-value="diff"
                    aria-pressed={mode === "diff"}
                    onClick={() => setMode("diff")}
                  >
                    Diff
                  </button>
                </div>
                <button
                  className="workbench-save"
                  data-action="workbench.save"
                  data-target={active.path}
                  disabled={busy || !file || draft === file.content}
                  onClick={() => void save()}
                >
                  <StudioIcon name="check" />
                  {t("保存")}
                </button>
              </div>
              {failed && (
                <div className="workbench-error" role="alert">
                  {failed}
                </div>
              )}
              {busy && !file ? (
                <div className="workbench-empty">{t("正在读取…")}</div>
              ) : mode === "diff" ? (
                <div className="workbench-diff">
                  <DiffView path={active.path} diff={diff} />
                </div>
              ) : file && file.path === active.path ? (
                <Suspense fallback={<div className="workbench-empty">{t("正在读取…")}</div>}>
                  <CodeEditor
                    key={active.path}
                    path={active.path}
                    value={draft}
                    dark={scheme === "dark"}
                    onChange={setDraft}
                    onSave={() => void save()}
                  />
                </Suspense>
              ) : null}
            </>
          )}
        </main>
        <aside className="workbench-explorer">
          <div className="workbench-explorer-head">
            <span>{t("资源管理器")}</span>
            <small>{files.length + directories.length}</small>
            {/* Beside the files rather than in settings: this is the one place
                the workspace is already what you are looking at. */}
            <button
              className="workbench-open-editor"
              data-action="workspace.editor"
              title={editorNote || t("在代码编辑器中打开工作区")}
              aria-label={t("在代码编辑器中打开工作区")}
              onClick={openEditor}
            >
              <StudioIcon name="code" />
            </button>
          </div>
          <label className="workbench-search">
            <StudioIcon name="search" />
            <input
              aria-label={t("搜索文件")}
              data-action="workbench.search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("搜索文件")}
            />
            {query && (
              <button data-action="workbench.clear-search" aria-label={t("清除搜索")} onClick={() => setQuery("")}>
                <StudioIcon name="close" />
              </button>
            )}
          </label>
          {/* What differs, without opening anything. The tree marks a changed
              file only once its folder is expanded, so a change three levels
              down was invisible until someone went looking for it. */}
          {changes.length > 0 && (
            <div className="workbench-changes">
              <div className="workbench-explorer-head">
                <span>{t("改动")}</span>
                <small>{changes.length}</small>
              </div>
              {changes.map((item) => (
                <button
                  className="workbench-change-row"
                  data-action="workbench.diff"
                  data-target={item.path}
                  key={`c:${item.path}`}
                  title={item.path}
                  aria-current={active?.kind === "file" && active.path === item.path && mode === "diff" ? "page" : undefined}
                  onClick={() => {
                    openFile(item.path);
                    setMode("diff");
                  }}
                >
                  <em data-status={item.status}>{item.status || "M"}</em>
                  <span>{item.path.slice(item.path.lastIndexOf("/") + 1)}</span>
                  {/* Absent counts print nothing: a binary file did not change
                      by zero lines, git simply did not count it. */}
                  {item.insertions !== undefined && <i data-io="up">+{item.insertions}</i>}
                  {item.deletions !== undefined && <i data-io="down">-{item.deletions}</i>}
                </button>
              ))}
            </div>
          )}
          <div className="workbench-filelist">
            {rows.map((row) =>
              row.kind === "folder" ? (
                <button
                  className="workbench-tree-row"
                  data-action="workbench.folder"
                  data-target={row.path}
                  key={`d:${row.path}`}
                  style={{ paddingInlineStart: 8 + row.depth * 14 }}
                  aria-expanded={!collapsed.has(row.path)}
                  onClick={() => void toggleFolder(row.path)}
                >
                  <StudioIcon
                    name={collapsed.has(row.path) ? "chevron" : "down"}
                  />
                  <StudioIcon name="folder" />
                  <span>{row.name}</span>
                </button>
              ) : (
                <button
                  className="workbench-tree-row"
                  data-action="workbench.file"
                  data-target={row.path}
                  key={row.path}
                  style={{ paddingInlineStart: 8 + row.depth * 14 }}
                  title={row.path}
                  aria-current={
                    active?.kind === "file" && active.path === row.path
                      ? "page"
                      : undefined
                  }
                  onClick={() => openFile(row.path)}
                >
                  <span />
                  <StudioIcon name="file" />
                  <span>{row.name}</span>
                  {changes.find((item) => item.path === row.path) && (
                    <em>
                      {changes.find((item) => item.path === row.path)?.status ||
                        "M"}
                    </em>
                  )}
                </button>
              ),
            )}
            {!rows.length && (
              <p className="workbench-no-files">{t("没有匹配的文件")}</p>
            )}
          </div>
        </aside>
      </div>
    </section>
  );
}
