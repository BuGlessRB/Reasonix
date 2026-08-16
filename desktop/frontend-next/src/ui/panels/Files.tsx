import type { Item } from "../../state/session";
import type { WorkspaceChanges } from "../../port/port";
import { argOf, shortArgs, splitPath } from "../args";
import { t } from "../../i18n";

// One row per file, not per call. Editing the same file four times is one
// pending change with four edits' worth of lines in it; a row per call turned
// the block into a log of what happened rather than what is waiting for review.
interface Change {
  path: string;
  added: number;
  removed: number;
  edits: number;
}

export function pending(items: Item[]): Change[] {
  const by = new Map<string, Change>();
  const take = (path: string) => {
    let c = by.get(path);
    if (!c) by.set(path, (c = { path, added: 0, removed: 0, edits: 0 }));
    return c;
  };
  for (const i of items) {
    if (i.t !== "tool") continue;
    const call = i.tool;
    if (call.added == null && call.removed == null) continue;
    // Falling back to the tool's own name printed "edit_file" where a path
    // belongs; the raw argument is the path whenever it is not JSON.
    const path = argOf(call.args, "path", "file_path") || shortArgs(call.args ?? "") || call.name;
    // A rename carries the change with it: the old path is no longer pending.
    if (call.name === "move_file") {
      const from = argOf(call.args, "from", "old_path", "source");
      if (from) by.delete(from);
    }
    const c = take(path);
    c.added += call.added ?? 0;
    c.removed += call.removed ?? 0;
    c.edits++;
  }
  return [...by.values()];
}

// The filename is the part being read; a deep directory must not push it out
// under an end-ellipsis. Keep the last two segments and mark the elision, with
// the whole path on the row's title for when it matters.
function elide(dir: string): string {
  const parts = dir.split("/").filter(Boolean);
  return parts.length <= 2 ? dir : `…/${parts.slice(-2).join("/")}/`;
}

const MARK: Record<string, string> = { A: "新增", "??": "新增", D: "已删除", R: "重命名" };

// A rail panel is a read on the run, not a full manifest. A session that edits
// thousands of files would otherwise put a row per file in the DOM — at 4000
// turns that panel alone was 28k nodes, an order of magnitude more than the
// whole transcript. The count in the header stays honest about the total.
const SHOWN = 60;

export function Files({ changes, yolo, tree }: { changes: Change[]; yolo: boolean; tree: WorkspaceChanges | null }) {
  // git is the authority when there is one: a path the session touched but the
  // tree no longer reports has been undone — created then removed, or edited
  // then reverted — and listing it keeps a change pending that does not exist.
  const byPath = tree?.repo ? new Map(tree.changes.map((c) => [c.path, c.status])) : null;
  // git filters, it does not supply: a repository is usually dirty for reasons
  // that have nothing to do with this session, and listing all of it buried the
  // agent's own edits under a page of files at +0 −0.
  const files = changes
    .filter((f) => !byPath || byPath.has(f.path))
    .map((f) => ({ ...f, mark: MARK[byPath?.get(f.path) ?? ""] ?? "" }));
  // The tail is the recent end: early files in a long run have been looked at
  // already, and the ones still moving are the ones worth a row.
  const shown = files.length > SHOWN ? files.slice(-SHOWN) : files;

  return (
    <div className="block" data-b="files">
      <div className="lbl">
        <span>{t(yolo ? "改动 · 已放行" : "待审改动")}</span>
        <span className="c">{files.length}</span>
      </div>
      <div className="files">
        {files.length === 0 && <span className="empty">{t("尚无改动")}</span>}
        {shown.length < files.length && (
          <span className="empty">{t("更早的 {n} 个未列出", { n: files.length - shown.length })}</span>
        )}
        {shown.map((f) => {
          const [dir, name] = splitPath(f.path);
          return (
            <div className="file" key={f.path} title={f.edits > 1 ? `${f.path} · 改了 ${f.edits} 次` : f.path}>
              <span className="p">
                {dir && <span className="d">{elide(dir)}</span>}
                <span className="f">{name}</span>
              </span>
              <span className="n">
                {f.mark && <span className="mk">{t(f.mark)}</span>}
                <span className="a">+{f.added}</span> <span className="r">−{f.removed}</span>
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
