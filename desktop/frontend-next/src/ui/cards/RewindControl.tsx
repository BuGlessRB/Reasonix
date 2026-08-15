import { useRef, useState } from "react";
import type { Checkpoint, RewindPlan, RewindResult, RewindScope } from "../../port/port";
import { useDismiss } from "../dismiss";

// Only edit-tool writes are snapshotted. A turn that also ran shell commands
// therefore has gaps the restore cannot reach, and the kernel refuses such a
// plan until it has been shown — so the gaps are what the second stage displays,
// read from the plan rather than written here as a standing caveat.
const SCOPES: { value: RewindScope; label: string; files: boolean }[] = [
  { value: "both", label: "代码和对话", files: true },
  { value: "code", label: "只还原代码", files: true },
  { value: "conversation", label: "只回退对话", files: false },
];

const GAP_REASONS: Record<string, string> = {
  bash_side_effect: "bash 命令改的东西没有快照",
};

type Stage =
  | { at: "closed" }
  | { at: "menu" }
  | { at: "working" }
  | { at: "confirm"; plan: RewindPlan }
  // The undo is only reachable while this stage holds the transaction id, which
  // is why the menu stays open after a commit instead of closing on success.
  | { at: "done"; tx: string; files: number }
  | { at: "failed"; why: string };

export function RewindControl({
  cp,
  onPrepare,
  onCommit,
  onUndo,
}: {
  cp: Checkpoint;
  onPrepare: (turn: number, scope: RewindScope) => Promise<RewindPlan>;
  onCommit: (planId: string) => Promise<RewindResult>;
  onUndo: (transactionId: string) => Promise<void>;
}) {
  const [stage, setStage] = useState<Stage>({ at: "closed" });
  const wrap = useRef<HTMLDivElement>(null);
  useDismiss(stage.at !== "closed", wrap, () => setStage({ at: "closed" }));

  const fail = (e: unknown) => setStage({ at: "failed", why: e instanceof Error ? e.message : String(e) });

  const pick = (scope: RewindScope) => {
    setStage({ at: "working" });
    onPrepare(cp.turn, scope)
      .then((plan) => {
        // Nothing to consent to: apply it straight away.
        if (!plan.requiresConfirmation) return onCommit(plan.planId).then((r) => settle(r, plan.fileCount));
        setStage({ at: "confirm", plan });
      })
      .catch(fail);
  };

  const commit = (plan: RewindPlan) => {
    setStage({ at: "working" });
    onCommit(plan.planId)
      .then((r) => settle(r, plan.fileCount))
      .catch(fail);
  };

  // A rewind the kernel can still reverse leaves the offer on screen; one it
  // cannot just closes, because an undo row that fails is worse than none.
  const settle = (result: RewindResult, files: number) => {
    const tx = result.undoAvailable ? (result.transactionId ?? "") : "";
    setStage(tx ? { at: "done", tx, files: result.deleted?.length ?? files } : { at: "closed" });
  };

  const undo = (tx: string) => {
    setStage({ at: "working" });
    onUndo(tx)
      .then(() => setStage({ at: "closed" }))
      .catch(fail);
  };

  return (
    <div className="stepctl rewind" ref={wrap}>
      <div className="picker">
        <button
          aria-haspopup="menu"
          aria-expanded={stage.at !== "closed"}
          title="把工作区和对话退回这条消息之前"
          onClick={() => setStage(stage.at === "closed" ? { at: "menu" } : { at: "closed" })}
        >
          ↩ 回到这里
        </button>
        <div className="menu projmenu" role="menu" hidden={stage.at === "closed"}>
          {stage.at === "menu" &&
            SCOPES.filter((s) => cp.files > 0 || !s.files).map((s) => (
              <button className="mi" role="menuitem" key={s.value} onClick={() => pick(s.value)}>
                <span className="dot" />
                <span className="tx">
                  <span className="lb">{s.label}</span>
                </span>
                {s.files && <span className="rt">{cp.files} 个文件</span>}
              </button>
            ))}
          {stage.at === "menu" && cp.files === 0 && (
            <div className="mi plain">
              <span className="dot" />
              <span className="tx">
                <span className="lb">这一轮没有改动文件</span>
              </span>
            </div>
          )}
          {stage.at === "working" && (
            <div className="mi plain">
              <span className="dot" />
              <span className="tx">
                <span className="lb">正在还原…</span>
              </span>
            </div>
          )}
          {stage.at === "confirm" && (
            <>
              <div className="mi plain">
                <span className="dot" />
                <span className="tx">
                  <span className="lb">这一轮有改动还原不了</span>
                  <span className="ds">
                    {(stage.plan.coverageGaps ?? [])
                      .map((g) => GAP_REASONS[g.reason] ?? g.detail)
                      .join("；") || "部分改动不在快照内"}
                  </span>
                </span>
              </div>
              <div className="div" />
              <button className="mi" role="menuitem" onClick={() => commit(stage.plan)}>
                <span className="dot" />
                <span className="tx">
                  <span className="lb">仍然还原剩下的</span>
                </span>
                <span className="rt">{stage.plan.fileCount} 个文件</span>
              </button>
              <button className="mi plain" role="menuitem" onClick={() => setStage({ at: "closed" })}>
                <span className="dot" />
                <span className="tx">
                  <span className="lb">取消</span>
                </span>
              </button>
            </>
          )}
          {stage.at === "done" && (
            <>
              <div className="mi plain">
                <span className="dot" />
                <span className="tx">
                  <span className="lb">已还原 {stage.files} 个文件</span>
                </span>
              </div>
              <div className="div" />
              <button className="mi" role="menuitem" onClick={() => undo(stage.tx)}>
                <span className="dot" />
                <span className="tx">
                  <span className="lb">撤销这次还原</span>
                </span>
              </button>
            </>
          )}
          {stage.at === "failed" && (
            <div className="mi plain">
              <span className="dot" />
              <span className="tx">
                <span className="lb">没能还原</span>
                <span className="ds">{stage.why}</span>
              </span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
