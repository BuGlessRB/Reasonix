import { t } from "../i18n";
import type { ModelEntry } from "../port/port";

// Only the ladder the kernel would accept for the model in hand. A fixed list
// here offered rungs a given model does not have, and picking one looked like
// the control was dead: the request was refused downstream, with nothing on the
// composer to say so. The kernel's list already opens with "auto" — prepending
// another one put the same rung in the menu twice. The fallback is for a model
// that declares nothing at all, and only then.
const EFFORT_FALLBACK = ["auto", "low", "medium", "high", "xhigh", "max"];

export function effortsFor(models: ModelEntry[], ref?: string): string[] {
  const model = models.find((m) => m.ref === ref);
  // A model that is in the list and names no levels has none: the host omits
  // the field exactly when it would refuse every level but auto. Filling that
  // in from the fallback is what put a full ladder in front of a relay whose
  // every rung came back refused. The fallback is for a list not answered yet.
  if (model) return model.efforts ?? [];
  return EFFORT_FALLBACK;
}

export function effortReading(value?: string): string {
  const id = (value || "auto").toLowerCase();
  const label = ({ auto: "自动", low: "快速", medium: "平衡", high: "深入", xhigh: "极致", max: "极致" } as Record<string, string>)[id] ?? id;
  const raw = id === "auto" ? "Auto" : id.charAt(0).toUpperCase() + id.slice(1);
  return `${t(label)} · ${raw}`;
}

export function effortLabel(value: string): string {
  const id = value.toLowerCase();
  return t(({ auto: "自动", disabled: "关闭", low: "快速", medium: "平衡", high: "深入", xhigh: "极致", max: "极致" } as Record<string, string>)[id] ?? id);
}

export function effortApi(value: string): string {
  const id = value.toLowerCase();
  return id === "xhigh" ? "XHigh" : id.charAt(0).toUpperCase() + id.slice(1);
}

export function effortDescription(value: string): string {
  const id = value.toLowerCase();
  return t(({
    auto: "使用模型默认或自适应策略，按任务复杂度调整",
    disabled: "不发送推理强度参数，使用服务端默认设置",
    low: "轻量思考，适合改写、提取和明确的小任务",
    medium: "兼顾响应速度与可靠性，适合大多数任务",
    high: "投入更多时间分析复杂上下文与执行方案",
    xhigh: "用于最复杂的问题，等待时间与消耗最高",
    max: "用于最复杂的问题，等待时间与消耗最高",
  } as Record<string, string>)[id] ?? value);
}

/** The rows the effort picker offers. An endpoint that reported no levels has
 *  not said "none" — it has said nothing, so rather than invent a rung the one
 *  row here points at where the capability is declared. */
export function effortMenu(efforts: string[], modelLabel: string, onDeclare: string) {
  const declared = efforts.length > 0;
  return [
    { value: "__effort-heading", label: t("推理强度"), right: modelLabel, header: true },
    ...(declared ? [] : [{ value: onDeclare, label: t("声明推理档位"), desc: t("这个端点没有报告推理档位，中转站通常不转发这项能力。将打开该来源的编辑表单，可以选择思考参数或直接填写档位。") }]),
    ...efforts.map((value) => ({
      value,
      label: effortLabel(value),
      meta: effortApi(value),
      badge: value === "auto" ? t("推荐") : undefined,
      recommended: value === "auto",
      strength: value === "disabled" ? 0 : ({ auto: 1, low: 1, medium: 2, high: 3, xhigh: 4, max: 4 } as Record<string, number>)[value.toLowerCase()] ?? 1,
      desc: effortDescription(value),
    })),
    ...(declared ? [{ value: "__effort-note", label: t("仅显示当前模型实际支持的档位。"), right: t("按模型生效"), header: true }] : []),
  ];
}
