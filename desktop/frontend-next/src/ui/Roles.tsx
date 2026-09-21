import { useRef, useState } from "react";
import { t } from "../i18n";
import type { ModelEntry, RoleAssignments } from "../port/port";
import { useDismiss } from "./dismiss";

// Five jobs, one default. Rendering them as five equal dropdowns would only
// move the confusion from the model list to a role list, so the main model is
// the anchor and a role draws a branch off it only once it stops following.

type RoleKey = keyof RoleAssignments;

const ROLES: [RoleKey, string, string][] = [
  ["planner", "计划", "仅生成计划，不写入"],
  ["subagent", "子代理", "派发的子任务"],
  ["vision", "看图", "处理主模型无法识别的图片"],
  ["guardian", "复核", "独立复核本轮"],
];

interface Props {
  models: ModelEntry[];
  roles: RoleAssignments | null;
  main?: string;
  busy: string;
  onSet: (role: string, ref: string) => void;
}

export function Roles({ models, roles, main, busy, onSet }: Props) {
  const [open, setOpen] = useState<RoleKey | null>(null);
  const anchor = models.find((m) => m.ref === main);
  // Whichever model an attachment actually reaches: the vision role if one is
  // assigned, otherwise the sub-agent it would be handed to, otherwise the main
  // model. The note below reports whether that model reads images at all.
  const visionRef = roles?.vision || roles?.subagent || main;
  const visionModel = models.find((m) => m.ref === visionRef);
  const readable = visionModel?.vision === true;

  if (!roles) return <div className="empty">{t("无法读取角色分工。")}</div>;

  const following = ROLES.filter(([k]) => !roles[k]).length;

  return (
    <>
      <div className="band">
        <div className="anchor">
          <span className="cap">{t("对话 · 主模型")}</span>
          <span className="nm">{anchor?.model ?? main ?? "—"}</span>
          <span className="meta">
            {[anchor?.provider, following === ROLES.length ? t("所有角色均跟随主模型") : t("{n} 个角色跟随主模型", { n: following })]
              .filter(Boolean)
              .join(" · ")}
          </span>
        </div>
        <div className="fan">
          {ROLES.map(([key, name, tag]) => (
            <Slot
              key={key}
              name={t(name)}
              tag={t(tag)}
              set={roles[key]}
              models={models}
              busy={busy}
              open={open === key}
              onOpen={() => setOpen(open === key ? null : key)}
              onPick={(ref) => {
                setOpen(null);
                onSet(key, ref);
              }}
            />
          ))}
        </div>
      </div>
      <p className="note">
        {readable
          ? t("主模型看不了的图会交给 {model}，它读图，所以附件真的会被看到。", { model: visionModel?.model ?? "" })
          : t("主模型无法识别的图片当前无人处理 —— 会在发送前被丢弃。为「看图」指定一个带「读图」标签的模型即可接管。")}
      </p>
    </>
  );
}

function Slot({
  name, tag, set, models, busy, open, onOpen, onPick,
}: {
  name: string; tag: string; set: string; models: ModelEntry[]; busy: string;
  open: boolean; onOpen: () => void; onPick: (ref: string) => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  const chosen = models.find((m) => m.ref === set);
  useDismiss(open, box, onOpen);

  return (
    <div className="slotbox" ref={box}>
      <button
        className="slot"
        data-set={set ? "" : undefined}
        aria-expanded={open}
        aria-haspopup="listbox"
        disabled={busy !== ""}
        onClick={onOpen}
      >
        <span className="rolecopy"><span className="role">{name}</span><span className="tag">{tag}</span></span>
        <span className="val" key={set || "follow"}>{set ? (chosen?.model ?? set) : t("跟随主模型")}</span>
        <i className="slotchev" aria-hidden="true">⌄</i>
      </button>
      {/* Kept conditional: .mgrp rounds its corners with overflow:hidden, released
          via :has(.rpick) only while the menu is open. Mounting it always would
          defeat that clip for good. */}
      {open && (
        <div className="rpick" role="listbox" aria-label={t("{name}用哪个模型", { name })}>
          <button role="option" data-action="roles.model" aria-selected={!set} data-cur={!set ? "" : undefined} onClick={() => onPick("")}>
            {t("跟随主模型")}
          </button>
          <div className="sep" />
          {models.map((m) => (
            <button
              key={m.ref}
              role="option"
                    data-action="roles.model"
              aria-selected={m.ref === set}
              data-cur={m.ref === set ? "" : undefined}
              onClick={() => onPick(m.ref)}
            >
              {m.model}
              <span className="sub">{m.vision ? t("读图") : m.provider}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
