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
 *  readings sit at the right end of the run rail, beside the other numbers
 *  about this turn, and each opens on hover the way the context reading does —
 *  one gesture for every summary on this row. */
export function DeckChips({ tasks, jobs, open, onOpen }: { tasks: Task[]; jobs: JobEntry[]; open: Deck; onOpen: (next: (was: Deck) => Deck) => void }) {
  const liveAgents = useMemo(() => agentsIn(tasks.filter((x) => x.running)), [tasks]);
  const liveJobs = useMemo(() => jobs.filter((j) => j.status === "running").length, [jobs]);
  if (tasks.length === 0 && jobs.length === 0) return null;

  return (
    <>
      {tasks.length > 0 && (
        <div className="studio-deck-anchor" data-open={open === "agents" ? "" : undefined}>
          <button
            type="button"
            className="studio-deckchip"
            data-action="deck.agents"
            data-live={liveAgents ? "" : undefined}
            aria-expanded={open === "agents"}
            aria-haspopup="dialog"
            onClick={() => onOpen((was) => (was === "agents" ? "" : "agents"))}
          >
            <StudioIcon name="branch" />
            <b>{liveAgents || agentsIn(tasks)}</b>
            <span>{t("子代理")}</span>
          </button>
          <div className="studio-deck-pop" role="dialog" aria-label={t("子代理")}>
            <Agents tasks={tasks} />
          </div>
        </div>
      )}
      {jobs.length > 0 && (
        <div className="studio-deck-anchor" data-open={open === "jobs" ? "" : undefined}>
          <button
            type="button"
            className="studio-deckchip"
            data-action="deck.jobs"
            data-live={liveJobs ? "" : undefined}
            aria-expanded={open === "jobs"}
            aria-haspopup="dialog"
            onClick={() => onOpen((was) => (was === "jobs" ? "" : "jobs"))}
          >
            <StudioIcon name="play" />
            <b>{liveJobs || jobs.length}</b>
            <span>{t("后台任务")}</span>
          </button>
          <div className="studio-deck-pop" role="dialog" aria-label={t("后台任务")}>
            <Jobs jobs={jobs} />
          </div>
        </div>
      )}
    </>
  );
}
