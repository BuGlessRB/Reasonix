import type { JobEntry, McpEntry, WorkspaceChanges } from "../port/port";
import type { ExtensionSurface } from "../port/wire";
import type { Item, Metrics as M, PlanStep } from "../state/session";
import { Plan } from "./Plan";
import { Cache } from "./panels/Cache";
import { Agents } from "./panels/Agents";
import { Jobs } from "./panels/Jobs";
import { Files } from "./panels/Files";
import { Mcp } from "./panels/Mcp";
import { Extensions } from "./panels/Extensions";

interface Props {
  metrics: M;
  plan: PlanStep[];
  items: Item[];
  jobs: JobEntry[];
  mcp: McpEntry[];
  rate: number;
  done: boolean;
  tree: WorkspaceChanges | null;
  yolo: boolean;
  onFold: () => void;
  onSettings: () => void;
  panels: ExtensionSurface[];
  onExtInvoke: (name: string) => void;
}

export function Metrics({ metrics, plan, items, jobs, mcp, rate, done, tree, yolo, onFold, onSettings, panels, onExtInvoke }: Props) {
  return (
    <>
      <div className="side-hd">
        <div className="lbl">度量</div>
        <button className="collapse" onClick={onFold} title="收起度量栏　⌘⇧\" aria-label="收起度量栏">
          ›
        </button>
      </div>
      <div className="scroll">
        <Cache metrics={metrics} rate={rate} done={done} />
        <Agents items={items} />
        <Jobs jobs={jobs} />
        <Mcp servers={mcp} onOpen={onSettings} />
        <Extensions panels={panels} onInvoke={onExtInvoke} />
        <Plan steps={plan} />
        <Files items={items} yolo={yolo} tree={tree} />
      </div>
    </>
  );
}
