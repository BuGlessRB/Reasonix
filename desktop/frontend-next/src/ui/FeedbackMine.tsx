import { useCallback, useEffect, useState, type ReactNode } from "react";
import { current, t } from "../i18n";
import { tx } from "../i18n/rich";
import { FEEDBACK_NEXT_VERSION, FEEDBACK_REPO_ISSUES, type FeedbackItem, type FeedbackMine as Mine, type FeedbackStatus } from "../port/feedback";
import type { AgentPort } from "../port/port";
import { CopyButton } from "./CopyButton";
import { feedbackFailure, type FeedbackFailure } from "./feedbackfailure";
import { StudioIcon, type StudioIconName } from "./StudioIcon";

export const STATUS_LABEL: Record<FeedbackStatus, string> = {
  received: "已收到",
  recorded: "已登记",
  in_progress: "处理中",
  fixed: "已修复",
  wontfix: "不予修复",
  duplicate: "重复",
};

const STATUS_ICON: Record<FeedbackStatus, StudioIconName> = {
  received: "clock",
  recorded: "list",
  in_progress: "refresh",
  fixed: "check",
  wontfix: "close",
  duplicate: "copy",
};

const CATEGORY_LABEL = { bug: "问题", idea: "建议", question: "疑问", other: "其他" } as const;

type StepState = "done" | "current" | "todo";

const STEP_STATE: Record<StepState, string> = { done: "已完成：", current: "当前：", todo: "尚未开始：" };

const SNIPPET_CUT = 80;

function snippet(text: string): string {
  return [...text].length >= SNIPPET_CUT ? `${text.trimEnd()}…` : text;
}

interface Step {
  id: string;
  label: ReactNode;
  state: StepState;
}

const ORDER: Record<FeedbackStatus, number> = { received: 0, recorded: 1, in_progress: 2, fixed: 3, wontfix: 3, duplicate: 3 };

function steps(item: FeedbackItem, issue: (n: number) => ReactNode): Step[] {
  const at = ORDER[item.status];
  const terminal = at === 3;
  const state = (i: number): StepState => (i < at || (terminal && i === at) ? "done" : i === at ? "current" : "todo");
  const recorded = item.issueNumber ? tx("已登记为 {issue}", { issue: issue(item.issueNumber) }) : t(STATUS_LABEL.recorded);
  const end: ReactNode =
    item.status === "fixed"
      ? item.resolvedVersion && item.resolvedVersion !== FEEDBACK_NEXT_VERSION
        ? t("已在 {version} 修复", { version: item.resolvedVersion })
        : t("已修复，将随下个版本发布")
      : item.status === "wontfix"
        ? t(STATUS_LABEL.wontfix)
        : item.status === "duplicate"
          ? item.duplicateOf
            ? tx("与 {issue} 重复", { issue: issue(item.duplicateOf) })
            : t(STATUS_LABEL.duplicate)
          : t("已解决");
  const out: Step[] = [
    { id: "received", label: t(STATUS_LABEL.received), state: state(0) },
    { id: "recorded", label: recorded, state: state(1) },
  ];
  if (item.status === "wontfix" || item.status === "duplicate") return [...out, { id: item.status, label: end, state: "done" }];
  return [...out, { id: "in_progress", label: t(STATUS_LABEL.in_progress), state: state(2) }, { id: terminal ? item.status : "resolved", label: end, state: state(3) }];
}

interface Props {
  port: AgentPort;
  onFile: (url: string) => void;
}

export function FeedbackMine({ port, onFile }: Props) {
  const [mine, setMine] = useState<Mine | null>(null);
  const [failure, setFailure] = useState<FeedbackFailure | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(() => {
    setLoading(true);
    port
      .myFeedback()
      .then((m) => {
        setMine(m);
        setFailure(null);
      })
      .catch((e) => setFailure(feedbackFailure(e)))
      .finally(() => setLoading(false));
  }, [port]);

  useEffect(load, [load]);

  const issue = (n: number): ReactNode => {
    if (!Number.isSafeInteger(n) || n <= 0) return `#${n}`;
    const url = FEEDBACK_REPO_ISSUES + String(n);
    return (
      <a href={url} rel="noopener noreferrer" data-action="feedback.link" onClick={(e) => { e.preventDefault(); onFile(url); }}>#{n}</a>
    );
  };

  const day = (iso: string) => new Date(iso).toLocaleDateString(current() === "zh" ? "zh-CN" : "en", { year: "numeric", month: "short", day: "numeric" });

  return (
    <div className="fbk-mine" aria-busy={loading}>
      <div className="fbk-mine-bar">
        <span className="fbk-hint">{t("每次打开这一页时刷新。状态来自对应的 GitHub 议题。")}</span>
        <button type="button" className="btn sm" data-action="feedback.refresh" disabled={loading} onClick={load}>
          <StudioIcon name="refresh" />
          {t("刷新列表")}
        </button>
      </div>

      {mine?.offline && (
        <div className="fbk-note" role="status" data-tone="info">
          <StudioIcon name="warning" />
          <span>{t("暂时连不上反馈服务。下面是保存在本机的记录，状态可能不是最新的。")}</span>
        </div>
      )}

      {mine?.items.some((i) => i.statusUnavailable) && (
        <div className="fbk-note" role="status" data-tone="info">
          <StudioIcon name="warning" />
          <span>{t("有些反馈是用这台电脑以前的身份发出的，已经查不到它们的最新状态。")}</span>
        </div>
      )}

      {failure && (
        <div className="fbk-note" role="alert" data-tone="error">
          <StudioIcon name="warning" />
          <span>{failure.message}</span>
          <button type="button" className="btn sm" data-action="feedback.retry-mine" onClick={load}>{t("重试")}</button>
        </div>
      )}

      {!mine && !failure && <p className="fbk-empty" role="status">{t("正在读取你的反馈…")}</p>}

      {mine && mine.items.length === 0 && (
        <p className="fbk-empty">{t("还没有提交过反馈。发出第一条之后，它的进展会出现在这里。")}</p>
      )}

      {mine && mine.items.length > 0 && (
        <ul className="fbk-list">
          {mine.items.map((item) => (
            <li key={item.receipt} className="fbk-item" data-status={item.status} data-stale={item.statusUnavailable ? "" : undefined}>
              <div className="fbk-item-hd">
                <code className="fbk-code">{item.receipt}</code>
                <CopyButton iconOnly text={item.receipt} label={t("复制回执号")} />
                <span className="fbk-meta">{t(CATEGORY_LABEL[item.category])} · {day(item.createdAt)}</span>
                {item.statusUnavailable ? (
                  <span className="fbk-chip" data-status="unavailable">{t("状态已无法追踪")}</span>
                ) : (
                  <span className="fbk-chip" data-status={item.status}>
                    <StudioIcon name={STATUS_ICON[item.status]} />
                    {t(STATUS_LABEL[item.status])}
                  </span>
                )}
              </div>
              <p className="fbk-snippet">{snippet(item.titleSnippet)}</p>
              {!item.statusUnavailable && <ol className="fbk-tl" aria-label={t("处理进展")}>
                {steps(item, issue).map((s) => (
                  <li key={s.id} data-state={s.state} aria-current={s.state === "current" ? "step" : undefined}>
                    <i aria-hidden="true" />
                    <span className="sr-only">{t(STEP_STATE[s.state])}</span>
                    <span>{s.label}</span>
                  </li>
                ))}
              </ol>}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
