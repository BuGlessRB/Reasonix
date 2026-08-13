import type { Item } from "../../state/session";
import { argOf, splitPath } from "../args";

export function Files({ items, yolo }: { items: Item[]; yolo: boolean }) {
  const files = items
    .filter((i): i is Extract<Item, { t: "tool" }> => i.t === "tool")
    .filter((i) => i.tool.added != null || i.tool.removed != null)
    .map((i) => ({
      id: i.id,
      path: argOf(i.tool.args, "path", "file_path") || i.tool.name,
      added: i.tool.added ?? 0,
      removed: i.tool.removed ?? 0,
    }));

  return (
    <div className="block" data-b="files">
      <div className="lbl">
        <span>{yolo ? "改动 · 已放行" : "待审改动"}</span>
        <span className="c">{files.length}</span>
      </div>
      <div className="files">
        {files.length === 0 && <span className="empty">尚无改动</span>}
        {files.map((f) => {
          const [dir, name] = splitPath(f.path);
          return (
            <div className="file" key={f.id}>
              <span className="p">
                <span className="d">{dir}</span>
                {name}
              </span>
              <span className="n">
                <span className="a">+{f.added}</span> <span className="r">−{f.removed}</span>
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
