import type { Item } from "../../state/session";
import { shortArgs } from "../args";

export function Agents({ items }: { items: Item[] }) {
  const tasks = items.filter(
    (i): i is Extract<Item, { t: "tool" }> => i.t === "tool" && i.tool.name === "task",
  );
  const live = tasks.filter((t) => t.running).length;

  return (
    <div className="block" data-b="agents">
      <div className="lbl">
        子代理<span className="c">{live ? `${live} 并行` : tasks.length || 0}</span>
      </div>
      <div className="agents">
        {tasks.length === 0 && <span className="empty">尚未派出</span>}
        {tasks.map((t) => (
          <div className="ag" key={t.id}>
            <i
              className="pip"
              data-settled={t.running ? undefined : ""}
              style={t.running ? { background: "var(--net)", animation: "tick 1.6s ease-in-out infinite" } : undefined}
            />
            <span className="nm">{t.tool.profile?.name || shortArgs(t.tool.args ?? "") || "task"}</span>
            <span className="rt">
              {t.running ? "运行中" : t.tool.durationMs ? `${(t.tool.durationMs / 1000).toFixed(0)}s` : "已交活"}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
