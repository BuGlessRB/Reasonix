import type { ExecutionState } from "../state/execution";
import type { Item } from "../state/session";
import type { TrajRow } from "../state/trajectory";
import type { TrajectoryAvailability } from "../port/wire";
import { Graph } from "./Graph";
import { Timeline } from "./Timeline";
import { Trajectory } from "./Trajectory";
import type { PaneView } from "./PaneNav";

/** The three readings of a run that answer "how did this go" rather than "what
 *  is it doing": the timeline, the event trajectory and the run graph. Each is
 *  mounted only while it is the view on screen — hidden, their rows were rebuilt
 *  on every streamed delta, drawn for nobody. */
export function PaneDetail({ view, run, items, traj, onOpen, onSave }: {
  view: PaneView;
  run: ExecutionState;
  items: Item[];
  traj: { rows: TrajRow[]; availability?: TrajectoryAvailability };
  onOpen: (call: string) => void;
  onSave: (name: string, content: string) => Promise<string | null>;
}) {
  return (
    <>
      <div className="scroll" data-pane="line" hidden={view !== "line"}>
        {view === "line" && <Timeline graph={run.graph} items={items} onOpen={onOpen} />}
      </div>
      <div className="scroll" data-pane="traj" hidden={view !== "traj"}>
        {view === "traj" && <Trajectory rows={traj.rows} availability={traj.availability} onSave={onSave} />}
      </div>
      <div className="scroll" data-pane="graph" hidden={view !== "graph"}>
        {view === "graph" && <Graph run={run} items={items} onOpen={onOpen} />}
      </div>
    </>
  );
}
