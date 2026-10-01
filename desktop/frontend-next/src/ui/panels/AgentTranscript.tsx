import { createPortal } from "react-dom";
import { seconds } from "../../i18n/format";
import { t } from "../../i18n";
import { nestLabel } from "../delegation";
import { useEscape } from "../dismiss";
import { toolFailed } from "../cards/outcome";
import { NestedCall } from "../cards/ToolCard";
import { StudioIcon } from "../StudioIcon";
import { agentName, type Task } from "./Agents";

export function AgentTranscript({ task, onClose }: { task: Task; onClose: () => void }) {
  useEscape(true, onClose);
  const { tool, children, running } = task;
  const status = running ? t("运行中") : toolFailed(tool) ? t("已中断") : t("已交付");
  const body = (
    <div className="agent-tx-scrim" data-action="agent.close" role="presentation" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="agent-tx" role="dialog" aria-modal="true" aria-label={t("子代理完整记录：{name}", { name: agentName(task) })} data-status={running ? "running" : "done"}>
        <div className="agent-tx-hd">
          <span className="who">{nestLabel(tool.profile?.name?.trim(), tool.profile?.count, children.length)}</span>
          <span className="rt">{[status, tool.durationMs ? seconds(tool.durationMs) : ""].filter(Boolean).join(" · ")}</span>
          <button type="button" className="agent-tx-x" aria-label={t("关闭")} onClick={onClose}>
            <StudioIcon name="close" />
          </button>
        </div>
        <div className="agent-tx-bd nest-bd">
          {children.length === 0 && <p className="mnote">{t("还没有记录到步骤")}</p>}
          {children.map((c) => (
            <NestedCall key={c.id} tool={c} whole />
          ))}
        </div>
        {tool.output && <div className="nest-ret">{tool.output}</div>}
      </div>
    </div>
  );
  return createPortal(body, document.body);
}
