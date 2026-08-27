import { t } from "../../i18n";
import type { PlanStep } from "../../state/session";
import type { WorkspaceChanges } from "../../port/port";
import { visible } from "./Files";
import type { Change, Stats } from "./derive";
import { Grp, Row } from "./kit";

interface Props {
  plan: PlanStep[];
  stats: Stats;
  changes: Change[];
  tree: WorkspaceChanges | null;
  yolo: boolean;
}

export function Overview({ plan, stats, changes, tree, yolo }: Props) {
  const total = plan.length;
  const done = plan.filter((s) => s.done).length;
  const files = visible(changes, tree).length;

  return (
    <Grp name={t("任务概览")}>
      <Row k={t("计划步数")} v={total ? `${done} / ${total}` : "—"} />
      <Row k={t("完成步数")} v={done} />
      <Row k={t("失败步数")} v={stats.failed} tone={stats.failed ? "err" : undefined} />
      <Row k={t("待确认")} v={stats.waiting} tone={stats.waiting ? "accent" : undefined} />
      <Row
        k={t("改动状态")}
        v={files ? <span className="lk">{t(yolo ? "已放行 {n}" : "待审 {n}", { n: files })}</span> : t("尚无改动")}
      />
    </Grp>
  );
}
