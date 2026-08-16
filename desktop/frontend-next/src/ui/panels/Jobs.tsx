import { useEffect, useState } from "react";
import { t } from "../../i18n";
import type { JobEntry } from "../../port/port";

export function Jobs({ jobs }: { jobs: JobEntry[] }) {
  const [, tick] = useState(0);

  useEffect(() => {
    if (!jobs.some((j) => j.status === "running")) return;
    const t = setInterval(() => tick((v) => v + 1), 1000);
    return () => clearInterval(t);
  }, [jobs]);

  return (
    <div className="block" data-b="jobs">
      <div className="lbl">
        {t("后台任务")}<span className="c">{jobs.length}</span>
      </div>
      <div className="jobs">
        {jobs.length === 0 && <span className="empty">{t("无")}</span>}
        {jobs.map((j) => {
          const running = j.status === "running";
          return (
            <div className="job" key={j.id} data-done={running ? undefined : ""}>
              <i
                className="pip"
                data-settled={running ? undefined : ""}
                style={running ? { background: "var(--net)", animation: "tick 1.6s ease-in-out infinite" } : undefined}
              />
              <span className="cmd">{j.label || j.id}</span>
              <span className="rt">{running ? `${Math.floor((Date.now() - j.startedAt) / 1000)}s` : j.status}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
