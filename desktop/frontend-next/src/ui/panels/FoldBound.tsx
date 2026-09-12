import { useEffect, useState } from "react";
import { t } from "../../i18n";
import { reason } from "../../i18n/kernel";
import type { AgentPort, CompactionSettings, ContextBreakdown } from "../../port/port";
import { foldModeOf, foldModeValue, type FoldMode } from "../foldbound";
import { tokens } from "../../i18n/format";

/** Where the fold point is changed, drawn beside the fold point itself.
 *
 *  The same three choices the settings sheet offers, because they are one
 *  intent and not two — but a reader who has just been told their 1M window
 *  folds at 160k is looking at this panel, not at a sheet two clicks away, and
 *  a setting that can only be found by people who already know it exists is
 *  one most people never find. The labels are the only thing that differs: the
 *  rail is a 245px column and the sheet's are sentences.
 *
 *  The bounds are read when the editor opens rather than carried on the gauge:
 *  the gauge redraws every turn, and this is asked for by hand. */
export function FoldBound({ port, onCtx, onDone }: {
  port: AgentPort;
  onCtx: (next: ContextBreakdown) => void;
  onDone: () => void;
}) {
  const [box, setBox] = useState<CompactionSettings | null>(null);
  // What the reader asked for, which leads the stored value: "custom" is a
  // choice before it is a number, and a mode derived from storage alone leaves
  // that click with nothing to show.
  const [choice, setChoice] = useState<FoldMode | null>(null);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let alive = true;
    port
      .compaction()
      .then((s) => {
        if (!alive) return;
        setBox(s);
        setDraft(String(s.soft_limit_tokens > 0 ? s.soft_limit_tokens : s.default_soft_limit));
      })
      .catch((e) => alive && setError(reason(e)));
    return () => {
      alive = false;
    };
  }, [port]);

  if (!box) return error ? <p className="ctxnote" data-lvl="warn">{error}</p> : null;

  // Writing rebuilds the runtime, so the gauge that comes back is the one the
  // session will actually fold against — not this reply's arithmetic.
  const save = async (value: number) => {
    if (busy || value === box.soft_limit_tokens) return;
    setBusy(true);
    setError("");
    try {
      setBox(await port.saveCompaction(value));
      setChoice(null);
      onCtx(await port.context());
    } catch (e) {
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  };

  const mode = choice ?? foldModeOf(box.soft_limit_tokens);
  const pick = (next: FoldMode) => {
    setError("");
    setChoice(next);
    const value = foldModeValue[next];
    if (value !== undefined) void save(value);
  };
  const commit = () => {
    const next = Number(draft.trim());
    if (!draft.trim()) return;
    if (!Number.isInteger(next) || next <= 0) {
      setError(t("请填一个大于 0 的整数。"));
      return;
    }
    setError("");
    void save(next);
  };

  const capacity = Math.round(box.context_window * box.ratio);
  return (
    <div className="ctxfold">
      <div className="seg" data-text role="group" aria-label={t("维护点")}>
        {([
          ["default", t("默认")],
          ["custom", t("自定义")],
          ["capacity", t("按容量")],
        ] as [FoldMode, string][]).map(([m, label]) => (
          <button
            key={m}
            data-action="compaction.threshold"
            data-value={m}
            aria-pressed={mode === m}
            disabled={busy}
            onClick={() => pick(m)}
          >
            {label}
          </button>
        ))}
      </div>
      {mode === "custom" && (
        <div className="r">
          <input
            autoFocus
            inputMode="numeric"
            value={draft}
            disabled={busy}
            placeholder={String(box.default_soft_limit)}
            aria-label={t("维护点（tokens）")}
            data-action-keydown="compaction.threshold"
            onChange={(e) => setDraft(e.target.value.replace(/\D/g, ""))}
            onKeyDown={(e) => {
              if (e.key === "Enter") commit();
              if (e.key === "Escape") onDone();
            }}
          />
          <button data-action="compaction.threshold" data-value="custom" disabled={busy || !draft} onClick={commit}>
            {t("记录")}
          </button>
        </div>
      )}
      {error && <p className="ctxnote" data-lvl="warn">{error}</p>}
      {/* What the chosen mode does to this session, said as the thing it does
          rather than as the name of a setting. The rebuild rides one sentence
          for all three: written into each, it is the half the eye learns to
          skip, and it is the half that says the change may have to wait. */}
      <p className="ctxnote">
        {mode === "capacity"
          ? t("只按窗口容量整理，本会话即 {n}。", { n: tokens(capacity) })
          : mode === "custom"
            ? t("可见输入到达该值即整理。")
            : t("默认 {n}，与窗口大小无关。", { n: tokens(box.default_soft_limit) })}
      </p>
      <p className="ctxfine">{t("会重建运行时；任务运行中改不了。")}</p>
    </div>
  );
}
