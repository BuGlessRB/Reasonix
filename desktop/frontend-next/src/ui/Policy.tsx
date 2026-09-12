import { useRef, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AgentPort, ApprovalMode, Preset, SessionStatus } from "../port/port";
import { useDismiss } from "./dismiss";
import { Seg } from "./Seg";
import { deviates, policySummary } from "./policyview";
import { Warn } from "./glyphs";

// 这一轮怎么跑，是三个变量共同回答的一个问题。它们原来分处窗口顶栏（执行方式）
// 和输入框底栏（强度、批准），要知道下一轮会怎么执行得看两个地方。
const PRESETS: [Preset, string, string][] = [
  ["balanced", "均衡", "以模型判定任务完成为准，适用于日常任务"],
  ["delivery", "交付", "任何修改均需经过验证、复核与签收，缺一不可视为完成"],
];

// 从紧到松排，和闸门环的缺口一个方向。「不打扰」在内核是 permission.Deny：
// 它比「询问」更严，不是更松 —— 排在询问前面才不会被读反。
const APPROVALS: [ApprovalMode, string, string][] = [
  ["dontAsk", "不询问", "不显示审批请求；需要批准的操作一律不执行。"],
  ["ask", "询问", "每次执行操作前请求确认。"],
  ["auto", "自动", "低风险操作自动放行，写入操作仍需确认。"],
  ["yolo", "全部放行", "不再请求确认，仅在完全信任当前工作区时使用。"],
];

type Field = "preset" | "effort" | "approval";

interface Props {
  port: AgentPort;
  status: SessionStatus | null;
  // The rungs this model actually has, decided upstream from the kernel's own
  // list. An empty ladder means the endpoint names none, and the field is not
  // drawn rather than drawn dead.
  efforts: string[];
  onChanged: () => void;
}

// 内核是三个独立的 effect，界面把它们画在一起不该把它们并成一个调用 —— 那会让
// 「强度没换成」连坐掉已经换好的执行方式。
export function Policy({ port, status, efforts, onChanged }: Props) {
  const [open, setOpen] = useState(false);
  const wrap = useRef<HTMLDivElement>(null);
  const btn = useRef<HTMLButtonElement>(null);
  useDismiss(open, wrap, () => setOpen(false));

  // 每个字段各自记「问出去的是哪一档」和「上一次为什么被拒」。合成一个就回到了
  // 整排一起变灰：一个字段在等，另外两个仍然能改。
  const [asking, setAsking] = useState<Partial<Record<Field, string>>>({});
  const [refused, setRefused] = useState<Partial<Record<Field, string>>>({});

  const apply = (field: Field, value: string, call: () => Promise<void>) => {
    if (asking[field]) return;
    setAsking((a) => ({ ...a, [field]: value }));
    setRefused((r) => ({ ...r, [field]: "" }));
    call()
      .then(onChanged)
      .catch((e: unknown) => setRefused((r) => ({ ...r, [field]: reason(e) })))
      .finally(() => setAsking((a) => ({ ...a, [field]: "" })));
  };

  if (!status) return null;

  const preset = status?.preset;
  const eff = status?.effort || "auto";
  const apv = status?.toolApprovalMode ?? "ask";
  // The shelf reads committed fact, never the answer being waited on: a field
  // still in flight shows its pending state inside the open menu, and a summary
  // that moved first would say the session is running under something the
  // kernel has not accepted — and may refuse.
  const sum = policySummary(status, efforts.length > 0);
  const quiet = !deviates(sum);

  return (
    <div className="picker policy" ref={wrap} data-quiet={quiet ? "" : undefined}>
      <button
        ref={btn}
        className="mode plain"
        data-action="chrome.policy"
        aria-expanded={open}
        aria-label={quiet ? t("本轮执行策略") : undefined}
        title={sum.ready ? sum.reading : ""}
        onClick={() => setOpen((v) => !v)}
      >
        {quiet ? (
          <>
            <span className="ic" aria-hidden="true">
              <svg viewBox="0 0 16 16">
                <path d="M2.5 4.2h4M9.5 4.2h4M6.5 2.7v3M2.5 11.8h7M12.5 11.8h1M10.5 10.3v3" />
              </svg>
            </span>
            <span className="lb">{t("执行")}</span>
          </>
        ) : (
          <span className="vl">
            {sum.ready && (
              <>
                {[sum.preset, sum.effort].filter(Boolean).join(" · ")}
                {sum.approval && (
                  <>
                    {/* Only between two things. With preset and effort both at
                        baseline the approval is the whole reading. */}
                    {(sum.preset || sum.effort) && " · "}
                    {/* The one state allowed to break the quiet. Nothing else on
                        this shelf can be forgotten and still cost a workspace. */}
                    {/* Scoped names: a bare .risk in this stylesheet is already an MCP
                        risk row, and inheriting its display:flex and margin doubled
                        this pill's height in exactly the dangerous state. */}
                    <span className={sum.danger ? "polrisk" : undefined}>
                      {sum.danger && <i className="polwarn" aria-hidden><Warn /></i>}
                      {sum.approval}
                    </span>
                  </>
                )}
              </>
            )}
          </span>
        )}
      </button>
      <div className="menu modemenu polmenu" role="group" aria-label={t("本轮执行策略")} hidden={!open}>
        <Seg
          label={t("执行方式")}
          data-action="chrome.preset"
          text
          options={PRESETS.map(([value, lb, note]) => ({ value, label: t(lb), note: t(note) }))}
          current={preset}
          asking={asking.preset}
          refused={refused.preset}
          onClick={(v) => apply("preset", v, () => port.setPreset(v as Preset))}
        />
        {/* An endpoint that names no levels gets no ladder: a control whose
            every rung is refused downstream reads as a dead one. */}
        {efforts.length > 0 && (
          <Seg
            label={t("思考强度")}
            data-action="reasoning.effort"
            options={efforts.map((value) => ({ value, label: value }))}
            current={eff}
            asking={asking.effort}
            refused={refused.effort}
            onClick={(v) => apply("effort", v, () => port.setEffort(v))}
          />
        )}
        <Seg
          label={t("工具权限")}
          data-action="tool-approval.mode"
          text
          options={APPROVALS.map(([value, lb, note]) => ({ value, label: t(lb), note: t(note) }))}
          current={apv}
          asking={asking.approval}
          refused={refused.approval}
          onClick={(v) => apply("approval", v, () => port.setApprovalMode(v as ApprovalMode))}
        />
      </div>
    </div>
  );
}
