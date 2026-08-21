import type { Item } from "../../state/session";
import { seconds } from "../../i18n/format";
import { shortArgs } from "../args";
import { t } from "../../i18n";
import { agentsOf, isDelegation } from "../delegation";

export type Task = Extract<Item, { t: "tool" }>;

// A profile is the kernel's mark that the work left this context. Matching the
// tool name instead missed every delegation, because the model reaches task,
// fleet and the skill runners through use_capability — the name on the card is
// the proxy's, not the dispatcher's.
export const tasksOf = (items: Item[]): Task[] =>
  items.filter((i): i is Task => i.t === "tool" && isDelegation(i.tool));

// One fleet call is many sub-agents; counting cards would under-report it.
const agentsIn = (tasks: Task[]) => tasks.reduce((n, x) => n + agentsOf(x.tool), 0);

// Same reason the file panel caps its rows: the rail reports, it does not
// enumerate. Running delegates come first — a finished one is history.
const SHOWN = 40;

export function Agents({ tasks }: { tasks: Task[] }) {
  const live = agentsIn(tasks.filter((x) => x.running));
  const shown = tasks.length > SHOWN ? tasks.slice(-SHOWN) : tasks;

  return (
    <div className="block" data-b="agents">
      <div className="lbl">
        {t("子代理")}<span className="c">{live ? t("{n} 并行", { n: live }) : agentsIn(tasks)}</span>
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
            <span className="nm">
              {x.tool.profile?.name || shortArgs(x.tool.args ?? "") || "task"}
              {(x.tool.profile?.count ?? 1) > 1 && <b className="mult">×{x.tool.profile?.count}</b>}
            </span>
            <span className="rt">
              {x.running ? t("运行中") : x.tool.durationMs ? seconds(x.tool.durationMs, 0) : t("已交活")}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
