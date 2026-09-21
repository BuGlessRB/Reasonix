import { useEffect, useMemo, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type {
  AgentPort,
  BrowserTab,
  WorkspaceChange,
  WorkspaceFile,
} from "../port/port";
import { AgentBrowserPanel, ManualBrowserPanel } from "./BrowserPanel";
import { DiffView } from "./cards/DiffView";
import { LazyMarkdown } from "./LazyMarkdown";
import { StudioIcon } from "./StudioIcon";

type Surface =
  | { kind: "manual" }
  | { kind: "browser"; tab: BrowserTab }
  | { kind: "file"; path: string };
type TreeRow = {
  kind: "folder" | "file";
  path: string;
  name: string;
  depth: number;
};
const LANGUAGE: Record<string, string> = {
  ts: "typescript",
  tsx: "typescript",
  js: "javascript",
  jsx: "javascript",
  go: "go",
  py: "python",
  rs: "rust",
  java: "java",
  json: "json",
  css: "css",
  html: "html",
  md: "markdown",
  sh: "bash",
  ps1: "powershell",
  toml: "ini",
  yaml: "yaml",
  yml: "yaml",
  xml: "xml",
  sql: "sql",
};

function keyOf(s: Surface) {
  return s.kind === "manual"
    ? "manual"
    : `${s.kind}:${s.kind === "browser" ? s.tab.target : s.path}`;
}
function labelOf(s: Surface) {
  return s.kind === "manual"
    ? t("浏览器")
    : s.kind === "browser"
      ? s.tab.title || s.tab.url || t("空白页")
      : s.path.split("/").at(-1) || s.path;
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
  return rows.sort(
    (a, b) => a.path.localeCompare(b.path) || (a.kind === "folder" ? -1 : 1),
  );
}

export function WorkbenchPanel({
  port,
  tabs,
  manual,
  shown,
  scheme,
  changes,
  onCloseManual,
  onExternal,
}: {
  port: AgentPort;
  tabs: BrowserTab[];
  manual: boolean;
  shown: boolean;
  scheme: "light" | "dark";
  changes: WorkspaceChange[];
  onCloseManual: () => void;
  onExternal: (url: string) => void;
}) {
  const [files, setFiles] = useState<string[]>([]),
    [directories, setDirectories] = useState<string[]>([]),
    [openFiles, setOpenFiles] = useState<string[]>([]);
  const [query, setQuery] = useState(""),
    [selected, setSelected] = useState("");
  const [dismissed, setDismissed] = useState<Set<string>>(() => new Set()),
    [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const [mode, setMode] = useState<"file" | "diff">("file"),
    [editing, setEditing] = useState(false);
  const [file, setFile] = useState<WorkspaceFile | null>(null),
    [draft, setDraft] = useState(""),
    [diff, setDiff] = useState("");
  const [busy, setBusy] = useState(false),
    [failed, setFailed] = useState("");
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
  useEffect(() => {
    if (manual) setSelected("manual");
  }, [manual]);
  const surfaces = useMemo<Surface[]>(
    () =>
      [
        ...(manual ? [{ kind: "manual" } as const] : []),
        ...tabs.map((tab) => ({ kind: "browser", tab }) as const),
        ...openFiles.map((path) => ({ kind: "file", path }) as const),
      ].filter((s) => !dismissed.has(keyOf(s))),
    [manual, tabs, openFiles, dismissed],
  );
  const active =
    surfaces.find((s) => keyOf(s) === selected) ??
    surfaces.find((s) => s.kind === "browser" && s.tab.active) ??
    surfaces[0];
  const rows = useMemo(
    () => treeRows(files, directories, query, collapsed),
    [files, directories, query, collapsed],
  );
  useEffect(() => {
    if (!active || active.kind !== "file") return;
    let live = true;
    setBusy(true);
    setFailed("");
    setEditing(false);
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
  const openFile = (path: string) => {
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
  const close = (surface: Surface) => {
    const key = keyOf(surface);
    if (surface.kind === "manual") onCloseManual();
    else if (surface.kind === "file")
      setOpenFiles((v) => v.filter((p) => p !== surface.path));
    else setDismissed((v) => new Set(v).add(key));
    if (selected === key) setSelected("");
  };
  const save = async () => {
    if (!file || draft === file.content) return;
    setBusy(true);
    setFailed("");
    try {
      setFile(await port.saveWorkspaceFile({ ...file, content: draft }));
      setEditing(false);
    } catch (e) {
      setFailed(reason(e));
    } finally {
      setBusy(false);
    }
  };
  const language =
    active?.kind === "file"
      ? (LANGUAGE[active.path.split(".").at(-1)?.toLowerCase() ?? ""] ??
        "plaintext")
      : "plaintext";
  return (
    <section className="workbench" aria-label={t("工作台")}>
      <header className="workbench-tabs" role="tablist">
        {surfaces.map((surface) => (
          <div
            className="workbench-tab"
            key={keyOf(surface)}
            role="tab"
            aria-selected={surface === active}
          >
            <button
              className="workbench-tab-pick"
              data-action="workbench.tab"
              data-target={keyOf(surface)}
              title={
                surface.kind === "browser" ? surface.tab.url : labelOf(surface)
              }
              onClick={() => setSelected(keyOf(surface))}
            >
              <StudioIcon name={surface.kind === "file" ? "file" : "globe"} />
              <span>{labelOf(surface)}</span>
            </button>
            <button
              className="workbench-tab-close"
              data-action="workbench.close"
              data-target={keyOf(surface)}
              aria-label={t("关闭 {name}", { name: labelOf(surface) })}
              title={t("关闭")}
              onClick={() => close(surface)}
            >
              <StudioIcon name="close" />
            </button>
          </div>
        ))}
      </header>
      <div className="workbench-body">
        <main className="workbench-canvas">
          {!active && (
            <div className="workbench-empty">
              <StudioIcon name="file" />
              <span>{t("从右侧选择文件，或打开浏览器")}</span>
            </div>
          )}
          {active?.kind === "manual" && (
            <ManualBrowserPanel
              onClose={onCloseManual}
              onExternal={onExternal}
              scheme={scheme}
            />
          )}
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
                {mode === "file" && (
                  <button
                    className="workbench-edit"
                    data-action="workbench.edit"
                    aria-pressed={editing}
                    onClick={() => setEditing((v) => !v)}
                  >
                    <StudioIcon name="edit" />
                    {editing ? t("预览") : t("编辑")}
                  </button>
                )}
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
              ) : editing ? (
                <textarea
                  data-action="workbench.edit"
                  aria-label={t("文件内容")}
                  className="workbench-editor"
                  value={draft}
                  onChange={(e) => setDraft(e.target.value)}
                  spellCheck={false}
                />
              ) : (
                <div className="workbench-code">
                  <LazyMarkdown text={`\`\`\`${language}\n${draft}\n\`\`\``} />
                </div>
              )}
            </>
          )}
        </main>
        <aside className="workbench-explorer">
          <div className="workbench-explorer-head">
            <span>{t("资源管理器")}</span>
            <small>{files.length + directories.length}</small>
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
