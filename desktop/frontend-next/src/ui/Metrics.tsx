import { memo, useCallback, useState } from "react";
import type { AgentPort, ContextBreakdown, JobEntry, McpEntry, WorkspaceChanges } from "../port/port";
import type { ExtensionSurface } from "../port/wire";
import type { Metrics as M } from "../state/session";
import { Agents } from "./panels/Agents";
import { Cache } from "./panels/Cache";
import { Context } from "./panels/Context";
import { Jobs } from "./panels/Jobs";
import { Files, visible } from "./panels/Files";
import { Mcp } from "./panels/Mcp";
import { Extensions } from "./panels/Extensions";
import { Runtime } from "./panels/Runtime";
import { Cost } from "./panels/Cost";
import type { Rail } from "./panels/derive";
import { swapping } from "./swap";
import { ChangePreview } from "./ChangePreview";

interface Props extends Rail {
  port: AgentPort;
  metrics: M;
  jobs: JobEntry[];
  mcp: McpEntry[];
  rate: number;
  done: boolean;
  tree: WorkspaceChanges | null;
  ctx: ContextBreakdown | null;
  yolo: boolean;
  onSettings: () => void;
  panels: ExtensionSurface[];
  // Composed views that resolved to the rail — the default place for a standing
  // surface nobody assigned elsewhere.
  views?: ExtensionSurface[];
  onExtInvoke: (name: string) => void;
  onMoveSurface?: (ext: ExtensionSurface, slot: string) => void;
}

// Progress, duration, cost and context are the head card's; what is left here
// is the composition behind them. Two sections answer "is this run healthy",
// and three more speak only when they have something to report. Everything else is a diagnosis — true, and not
// worth a permanent nine-panel wall that a glance has to skip six of. The walk
// behind all of it happens once, in the pane, and arrives here derived.
export const Metrics = memo(function Metrics({
  port,
  metrics,
  tasks,
  changes,
  stats,
  jobs,
  mcp,
  rate,
  done,
  tree,
  ctx,
  yolo,
  onSettings,
  panels,
  views,
  onExtInvoke,
  onMoveSurface,
}: Props) {
  // The rail owns which change is open, so it closes with the pane rather than
  // outliving it in a window-level store.
  const [openPath, setOpenPath] = useState<string | null>(null);
  // Both ends go through the swap: the preview is conditionally rendered, so
  // without one it has an entrance and no way out — it stops existing, which
  // reads as the window flinching rather than as the panel leaving.
  const openPreview = useCallback((path: string) => swapping(() => setOpenPath(path), "cpv"), []);
  const closePreview = useCallback(() => swapping(() => setOpenPath(null), "cpv"), []);
  return (
    <>
      <div className="scroll">
        {/* 顺序照设计稿：花了多少、省下多少、窗口装了什么、动过哪些文件 ——
            四个“这一趟在付什么代价”的问题排在一起，子代理和运行详情接在后面。 */}
        <Cache metrics={metrics} done={done} />
        <Cost metrics={metrics} />
        <Context ctx={ctx} row={false} legend />
        <Agents tasks={tasks} />
        <Runtime rate={rate} done={done} stats={stats} files={visible(changes, tree).length} />
        <Mcp servers={mcp} onOpen={onSettings} />
        <Extensions panels={panels} views={views} onInvoke={onExtInvoke} onMove={onMoveSurface} />
        {/* 最后两块，和设计稿一样：前面那些回答“这一趟在怎么跑”，这两块
            回答“它在外面动了什么” —— 后者是跑完了回头看的东西，不是盯着看的。 */}
        <Files changes={changes} yolo={yolo} tree={tree} open={openPath} onOpen={tree?.repo ? openPreview : undefined} />
        <Jobs jobs={jobs} />
      </div>
      {openPath && <ChangePreview port={port} path={openPath} onClose={closePreview} />}
    </>
  );
});
