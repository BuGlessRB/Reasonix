import { memo, useMemo } from "react";
import { t } from "../i18n";
import type { ContextBreakdown, JobEntry, McpEntry, WorkspaceChanges } from "../port/port";
import type { ExtensionSurface } from "../port/wire";
import type { Item, Metrics as M, PlanStep } from "../state/session";
import { Plan } from "./Plan";
import { Cache } from "./panels/Cache";
import { Context } from "./panels/Context";
import { Agents, tasksOf } from "./panels/Agents";
import { Jobs } from "./panels/Jobs";
import { Files, pending } from "./panels/Files";
import { Mcp } from "./panels/Mcp";
import { Extensions } from "./panels/Extensions";

interface Props {
  metrics: M;
  plan: PlanStep[];
  items: Item[];
  revision: number;
  jobs: JobEntry[];
  mcp: McpEntry[];
  rate: number;
  done: boolean;
  tree: WorkspaceChanges | null;
  ctx: ContextBreakdown | null;
  yolo: boolean;
  onFold: () => void;
  onSettings: () => void;
  panels: ExtensionSurface[];
  // Composed views that resolved to the rail — the default place for a standing
  // surface nobody assigned elsewhere.
  views?: ExtensionSurface[];
  onExtInvoke: (name: string) => void;
  onMoveSurface?: (ext: ExtensionSurface, slot: string) => void;
}

// Both panels read the tool cards and nothing else, so they are derived once per
// transcript revision rather than per streamed chunk. This wrapper exists to do
// that derivation outside the memo below, which is what keeps a whole rail of
// panels out of the frame budget while an answer streams.
export function Metrics({ items, revision, ...rest }: Props) {
  /* eslint-disable react-hooks/exhaustive-deps */
  const tasks = useMemo(() => tasksOf(items), [revision]);
  const changes = useMemo(() => pending(items), [revision]);
  /* eslint-enable react-hooks/exhaustive-deps */
  return <Rail {...rest} tasks={tasks} changes={changes} />;
}

const Rail = memo(function Rail({
  metrics,
  plan,
  tasks,
  changes,
  jobs,
  mcp,
  rate,
  done,
  tree,
  ctx,
  yolo,
  onFold,
  onSettings,
  panels,
  views,
  onExtInvoke,
  onMoveSurface,
}: Omit<Props, "items" | "revision"> & { tasks: ReturnType<typeof tasksOf>; changes: ReturnType<typeof pending> }) {
  return (
    <>
      <div className="side-hd">
        <div className="lbl">{t("度量")}</div>
        <button className="collapse" onClick={onFold} title={t("收起度量栏")} aria-label={t("收起度量栏")}>
          ›
        </button>
      </div>
      <div className="scroll">
        <Cache metrics={metrics} rate={rate} done={done} />
        <Context ctx={ctx} />
        <Agents tasks={tasks} />
        <Jobs jobs={jobs} />
        <Mcp servers={mcp} onOpen={onSettings} />
        <Extensions panels={panels} views={views} onInvoke={onExtInvoke} onMove={onMoveSurface} />
        <Plan steps={plan} />
        <Files changes={changes} yolo={yolo} tree={tree} />
      </div>
    </>
  );
});
