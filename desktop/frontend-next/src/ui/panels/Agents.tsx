import type { Item } from "../../state/session";
import { seconds } from "../../i18n/format";
import { shortArgs } from "../args";
import { t } from "../../i18n";

import { agentsIn } from "./derive";
import { Grp, Row } from "./kit";

export type Task = Extract<Item, { t: "tool" }>;

// Same reason the file panel caps its rows: the rail reports, it does not
// enumerate. Running delegates come first — a finished one is history.
const SHOWN = 40;

export function Agents({ tasks }: { tasks: Task[] }) {
  const live = agentsIn(tasks.filter((x) => x.running));
  const shown = tasks.length > SHOWN ? tasks.slice(-SHOWN) : tasks;

  return (
    <Grp name={t("子代理")} aside={live ? t("{n} 在跑", { n: live }) : undefined}>
      <Row
        k={t("子代理数量")}
        v={live ? <span className="lk">{t("{n} 并行", { n: live })}</span> : agentsIn(tasks)}
      />
      {tasks.length > 0 && (
        <div className="agents">
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
      )}
    </Grp>
  );
}
