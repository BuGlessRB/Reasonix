import { useState } from "react";
import { t } from "../../i18n";
import { reason } from "../../i18n/kernel";
import type { RewindPlan, RewindResult } from "../../port/port";

interface Props {
  diff: string;
  path?: string;
  onPrepare?: (path: string) => Promise<RewindPlan>;
  onCommit?: (planId: string, resolution?: string) => Promise<RewindResult>;
}

// Reverting is two steps on purpose. The first answers "is this file still the
// one the checkpoint captured"; only when it is not does the second need an
// answer from the reader, and asking before knowing would ask every time.
export function DiffView({ diff, path, onPrepare, onCommit }: Props) {
  const lines = diff.split("\n").filter((l) => l.length > 0);
  const [plan, setPlan] = useState<RewindPlan | null>(null);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [failed, setFailed] = useState("");
  const revertable = Boolean(path && onPrepare && onCommit && !done);
  const clash = plan?.conflicts?.[0];

  const prepare = async () => {
    if (!path || !onPrepare) return;
    setBusy(true);
    setFailed("");
    try {
      setPlan(await onPrepare(path));
    } catch (e) {
      setFailed(reason(e));
    } finally {
      setBusy(false);
    }
  };

  const commit = async (resolution?: string) => {
    if (!plan || !onCommit) return;
    setBusy(true);
    setFailed("");
    try {
      await onCommit(plan.planId, resolution);
      setPlan(null);
      setDone(true);
    } catch (e) {
      setFailed(reason(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="dif">
      <div className="dif-hd">
        <span>{path ?? t("改动")}</span>
        {done ? (
          <span className="ro">{t("已还原")}</span>
        ) : revertable && !plan ? (
          <button className="dif-act" disabled={busy} onClick={() => void prepare()}>
            {t(busy ? "…" : "还原这个文件")}
          </button>
        ) : (
          <span className="ro">{t("只读")}</span>
        )}
      </div>

      {plan && (
        <div className="dif-ask" data-clash={clash ? "" : undefined}>
          {clash ? (
            <>
              <div className="q">{t("这个文件在检查点之后又被改过了。")}</div>
              <div className="row">
                <button className="dif-act" disabled={busy} onClick={() => void commit("overwrite_checkpoint")}>
                  {t("用检查点的版本覆盖")}
                </button>
                <button className="dif-act ghost" disabled={busy} onClick={() => void commit("keep_current")}>
                  {t("保留现在的")}
                </button>
                <button className="dif-act ghost" onClick={() => setPlan(null)}>{t("取消")}</button>
              </div>
            </>
          ) : (
            <>
              <div className="q">{t("把这个文件还原到这次改动之前。")}</div>
              <div className="row">
                <button className="dif-act" disabled={busy} onClick={() => void commit()}>
                  {t(busy ? "正在还原…" : "还原")}
                </button>
                <button className="dif-act ghost" onClick={() => setPlan(null)}>{t("取消")}</button>
              </div>
            </>
          )}
        </div>
      )}
      {failed && <div className="dif-ask">{failed}</div>}

      {lines.map((l, i) => {
        const sign = l[0] === "+" || l[0] === "-" ? l[0] : " ";
        return (
          <div className="dl" key={i} data-d={sign === " " ? undefined : sign}>
            <span className="no">{i + 1}</span>
            <span className="sg">{sign}</span>
            <span className="cd">{l.slice(sign === " " ? 0 : 1)}</span>
          </div>
        );
      })}
    </div>
  );
}
