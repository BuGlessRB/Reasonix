import type { JobEntry } from "../port/port";
import type { Item, Metrics as M, PlanStep } from "../state/session";
import { Plan } from "./Plan";
import { Cache } from "./panels/Cache";
import { Agents } from "./panels/Agents";
import { Jobs } from "./panels/Jobs";
import { Files } from "./panels/Files";

interface Props {
  metrics: M;
  plan: PlanStep[];
  items: Item[];
  jobs: JobEntry[];
  rate: number;
  yolo: boolean;
  onFold: () => void;
}

export function Metrics({ metrics, plan, items, jobs, rate, yolo, onFold }: Props) {
  return (
    <>
      <div className="side-hd">
        <div className="lbl">度量</div>
        <button className="collapse" onClick={onFold} title="收起度量栏　⌘⇧\" aria-label="收起度量栏">
          ›
        </button>
      </div>
      <div className="scroll">
        <Cache metrics={metrics} rate={rate} />
        <Agents items={items} />
        <Jobs jobs={jobs} />
        <Plan steps={plan} />
        <Files items={items} yolo={yolo} />
      </div>
    </>
  );
}
