import { useState } from "react";
import { t } from "../../i18n";
import { reason } from "../../i18n/kernel";
import type { RewindPlan, RewindResult } from "../../port/port";

interface Props {
  diff: string;
  path?: string;
  /** The surface around this diff already names the file. */
  named?: boolean;
  onPrepare?: (path: string) => Promise<RewindPlan>;
  onCommit?: (planId: string, resolution?: string) => Promise<RewindResult>;
}

// Reverting is two steps on purpose. The first answers "is this file still the
// one the checkpoint captured"; only when it is not does the second need an
// answer from the reader, and asking before knowing would ask every time.

type Row = { sign: " " | "+" | "-"; text: string; no: number | null } | { skipped: number };

// Unified diff 的行号住在 @@ 头里。此前整段按 \n 切开、序号直接用下标，于是一处
// 改在第 205 行的改动在卡上标成 1、2、3 —— 而那个头本身还被当成代码渲染了出来。
// 空行也不再丢：缩进块的形状就是靠它们读出来的。
export function parseDiff(diff: string): Row[] {
  const out: Row[] = [];
  let oldNo = 0;
  let newNo = 0;
  let lastNew = 0;
  let sawHunk = false;
  for (const raw of diff.split("\n")) {
    const at = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(raw);
    if (at) {
      const start = Number(at[2]);
      if (sawHunk && start > lastNew + 1) out.push({ skipped: start - lastNew - 1 });
      oldNo = Number(at[1]);
      newNo = start;
      sawHunk = true;
      continue;
    }
    // 文件头不是内容，从来都不该出现在正文里。
    if (/^(diff |index |--- |\+\+\+ |new file|deleted file|similarity index|rename )/.test(raw)) continue;
    const c = raw[0];
    const sign: " " | "+" | "-" = c === "+" || c === "-" ? c : " ";
    const text = sign === " " ? raw : raw.slice(1);
    if (!sawHunk) {
      // 没有 @@ 头的裸片段：给不出真行号，就不给 —— 编一个比留空更坏。
      out.push({ sign, text, no: null });
      continue;
    }
    if (sign === "+") {
      out.push({ sign, text, no: newNo });
      lastNew = newNo++;
    } else if (sign === "-") {
      // 删掉的那一行在改动后的文件里不存在，所以这一格是空的。行号列自始至终
      // 是同一件事：跳过去要落在第几行。混进旧文件的行号，两种数字长得一样，
      // 读者没有办法分辨自己看的是哪一个文件。
      out.push({ sign, text, no: null });
      oldNo++;
    } else {
      out.push({ sign, text, no: newNo });
      lastNew = newNo++;
      oldNo++;
    }
  }
  return out;
}

export function DiffView({ diff, path, named, onPrepare, onCommit }: Props) {
  const lines = parseDiff(diff);
  const [plan, setPlan] = useState<RewindPlan | null>(null);
  const [busy, setBusy] = useState(false);
  const [outcome, setOutcome] = useState<"" | "reverted" | "kept" | "refused">("");
  const [failed, setFailed] = useState("");
  const revertable = Boolean(path && onPrepare && onCommit && !outcome);
  const clash = plan?.conflicts?.[0];

  const prepare = async () => {
    if (!path || !onPrepare) return;
    setBusy(true);
    setFailed("");
    try {
      const got = await onPrepare(path);
      // The kernel refuses some files outright — not session-owned, payload
      // expired, path unsafe — and says so on the plan. Offering the confirm
      // anyway posts an empty planId and answers "missing planId".
      if (!got.canFiles || !got.planId) {
        setOutcome("refused");
        setFailed(got.disabledReason || t("该文件无法从检查点还原"));
        return;
      }
      setPlan(got);
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
      const result = await onCommit(plan.planId, resolution);
      setPlan(null);
      // keep_current is the kernel's deliberate no-op: it answers OK and writes
      // nothing. Reporting that as "reverted" tells the reader the opposite of
      // what they just chose.
      setOutcome(result.written?.length || result.deleted?.length ? "reverted" : "kept");
    } catch (e) {
      setFailed(reason(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="dif">
      <div className="dif-hd">
        {/* The card's own headline already names the file; saying it again
            here truncates one fact twice. Where there is no headline — the
            change preview — this row is the only thing that names it. */}
        <span title={path ?? undefined}>{named ? "" : (path ?? t("改动"))}</span>
        {outcome === "reverted" ? (
          <span className="ro">{t("已还原")}</span>
        ) : outcome === "kept" ? (
          <span className="ro">{t("保留了当前版本")}</span>
        ) : revertable && !plan ? (
          <button className="dif-act" data-action="file-revert.prepare" disabled={busy} onClick={() => void prepare()}>
            {t(busy ? "…" : "还原该文件")}
          </button>
        ) : (
          <span className="ro">{t("只读")}</span>
        )}
      </div>

      {plan && (
        <div className="dif-ask" data-clash={clash ? "" : undefined}>
          {clash ? (
            <>
              <div className="q">{t("该文件在检查点之后再次被修改。")}</div>
              <div className="row">
                <button className="dif-act" data-action="file-revert.commit" data-value="overwrite_checkpoint" disabled={busy} onClick={() => void commit("overwrite_checkpoint")}>
                  {t("以检查点版本覆盖")}
                </button>
                <button className="dif-act ghost" data-action="file-revert.commit" data-value="keep_current" disabled={busy} onClick={() => void commit("keep_current")}>
                  {t("保留当前版本")}
                </button>
                <button className="dif-act ghost" onClick={() => setPlan(null)}>{t("取消")}</button>
              </div>
            </>
          ) : (
            <>
              {/* earliestRevisions(0): the preimage is the session's first, not
                  this turn's. Saying "before this change" would promise a
                  surgical undo and deliver a session-wide one. */}
              <div className="q">{t("将该文件还原至本次会话开始时的状态 —— 会话中对它的其他改动也会一并撤销。")}</div>
              <div className="row">
                <button className="dif-act" data-action="file-revert.commit" disabled={busy} onClick={() => void commit()}>
                  {t(busy ? "正在还原…" : "还原")}
                </button>
                <button className="dif-act ghost" onClick={() => setPlan(null)}>{t("取消")}</button>
              </div>
            </>
          )}
        </div>
      )}
      {failed && <div className="dif-ask">{failed}</div>}

      {/* 长行横滚在这一层，不在整块上：行号列跟着滚出去，读者就找不到自己在哪
          一行了。被改的那一段常常正好在行尾。 */}
      <div className="dlwrap">
        {lines.map((l, i) =>
          "skipped" in l ? (
            <div className="dhunk" key={i}>
              <span>⋯</span>
              <span>{t("跳过 {n} 行", { n: l.skipped })}</span>
            </div>
          ) : (
            <div className="dl" key={i} data-d={l.sign === " " ? undefined : l.sign}>
              <span className="no">{l.no ?? ""}</span>
              <span className="sg">{l.sign}</span>
              <span className="cd">{l.text}</span>
            </div>
          ),
        )}
      </div>
    </div>
  );
}
