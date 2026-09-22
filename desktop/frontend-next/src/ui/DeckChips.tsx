import { useMemo } from "react";
import type { JobEntry } from "../port/port";
import { t } from "../i18n";
import { StudioIcon } from "./StudioIcon";
import { agentsIn } from "./panels/derive";
import { Agents } from "./panels/Agents";
import { Jobs } from "./panels/Jobs";
import type { Task } from "./panels/Agents";

export type Deck = "" | "agents" | "jobs";

/** What is running on this session's behalf but is not in the transcript: work
 *  handed to a sub-agent, and processes left running in the background. The
 *  readings sit on the run rail and open the panel behind them, because a
 *  session can be delegating four ways and otherwise say nothing about it. */
export function DeckChips({ tasks, jobs, open, onOpen }: { tasks: Task[]; jobs: JobEntry[]; open: Deck; onOpen: (next: (was: Deck) => Deck) => void }) {
  const liveAgents = useMemo(() => agentsIn(tasks.filter((x) => x.running)), [tasks]);
  const liveJobs = useMemo(() => jobs.filter((j) => j.status === "running").length, [jobs]);
  if (tasks.length === 0 && jobs.length === 0) return null;

  return (
    <div className="studio-deckchips">
      {tasks.length > 0 && (
        <button
          type="button"
          className="studio-deckchip"
          data-action="deck.agents"
          data-live={liveAgents ? "" : undefined}
          aria-expanded={open === "agents"}
          onClick={() => onOpen((was) => (was === "agents" ? "" : "agents"))}
        >
          <StudioIcon name="branch" />
          <b>{liveAgents || agentsIn(tasks)}</b>
          <span>{t("子代理")}</span>
        </button>
      )}
      {jobs.length > 0 && (
        <button
          type="button"
          className="studio-deckchip"
          data-action="deck.jobs"
          data-live={liveJobs ? "" : undefined}
          aria-expanded={open === "jobs"}
          onClick={() => onOpen((was) => (was === "jobs" ? "" : "jobs"))}
        >
          <StudioIcon name="play" />
          <b>{liveJobs || jobs.length}</b>
          <span>{t("后台任务")}</span>
        </button>
      )}
    </div>
  );
}

/** The panel behind a chip. It opens upward out of the rail the reading sits
 *  on, so the summary and its detail stay one object. */
export function Deck({ open, tasks, jobs }: { open: Deck; tasks: Task[]; jobs: JobEntry[] }) {
  return (
    <div className="studio-deck" data-open={open ? "" : undefined}>
      {open === "agents" && <Agents tasks={tasks} />}
      {open === "jobs" && <Jobs jobs={jobs} />}
    </div>
  );
}
