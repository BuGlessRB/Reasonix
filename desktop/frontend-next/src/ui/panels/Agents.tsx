import type { Item } from "../../state/session";
import { seconds } from "../../i18n/format";
import { shortArgs } from "../args";
import { t } from "../../i18n";

export type Task = Extract<Item, { t: "tool" }>;

export const tasksOf = (items: Item[]): Task[] =>
  items.filter((i): i is Task => i.t === "tool" && i.tool.name === "task");

// Same reason the file panel caps its rows: the rail reports, it does not
// enumerate. Running delegates come first — a finished one is history.
const SHOWN = 40;

export function Agents({ tasks }: { tasks: Task[] }) {
  const live = tasks.filter((x) => x.running).length;
  const shown = tasks.length > SHOWN ? tasks.slice(-SHOWN) : tasks;

  return (
    <div className="block" data-b="agents">
      <div className="lbl">
        {t("子代理")}<span className="c">{live ? t("{n} 并行", { n: live }) : tasks.length || 0}</span>
      </div>
      <div className="agents">
        {tasks.length === 0 && <span className="empty">{t("尚未派出")}</span>}
        {shown.map((x) => (
          <div className="ag" key={x.id}>
            <i
              className="pip"
              data-settled={x.running ? undefined : ""}
              style={x.running ? { background: "var(--net)", animation: "tick 1.6s ease-in-out infinite" } : undefined}
            />
            <span className="nm">{x.tool.profile?.name || shortArgs(x.tool.args ?? "") || "task"}</span>
            <span className="rt">
              {x.running ? t("运行中") : x.tool.durationMs ? seconds(x.tool.durationMs, 0) : t("已交活")}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
