import { useRef, useState } from "react";
import type { ModelEntry, RoleAssignments } from "../port/port";
import { useDismiss } from "./dismiss";

// Five jobs, one default. Rendering them as five equal dropdowns would only
// move the confusion from the model list to a role list, so the main model is
// the anchor and a role draws a branch off it only once it stops following.

type RoleKey = keyof RoleAssignments;

const ROLES: [RoleKey, string, string][] = [
  ["planner", "计划", "只读地出计划"],
  ["subagent", "子代理", "派出去的活"],
  ["guardian", "复核", "独立审这一轮"],
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
  // A vision-capable subagent is what makes an attached image reach a model at
  // all when the main one is text-only. Saying so is honest; drawing a switch
  // for a model the kernel cannot yet name would not be.
  const visionRef = roles?.subagent || main;
  const visionModel = models.find((m) => m.ref === visionRef);

  if (!roles) return <div className="empty">读不到分工。</div>;

  const following = ROLES.filter(([k]) => !roles[k]).length;

  return (
    <>
      <div className="band">
        <div className="anchor">
          <span className="cap">对话 · 主模型</span>
          <span className="nm">{anchor?.model ?? main ?? "—"}</span>
          <span className="meta">
            {[anchor?.provider, following === ROLES.length ? "所有分工都跟着它" : `${following} 个分工跟着它`]
              .filter(Boolean)
              .join(" · ")}
          </span>
        </div>
        <div className="fan">
          {ROLES.map(([key, name, tag]) => (
            <Slot
              key={key}
              name={name}
              tag={tag}
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
          <div className="slotbox">
            <div className="slot" data-borrowed="">
              <i className="node" />
              <span className="role">看图</span>
              <span className="val" key={visionRef}>{visionModel?.model ?? "—"}</span>
              <span className="tag">{visionModel?.vision ? "借用子代理" : "读不了图"}</span>
            </div>
          </div>
        </div>
      </div>
      <p className="note">
        {visionModel?.vision
          ? "图会走子代理，而这个子代理模型正好读图，所以附件真的会被看到。"
          : "「看图」还没有自己的开关：图交给子代理，用的是子代理模型。现在这个模型不读图，所以附上的图会在发出去之前被丢掉 —— 把子代理换成一个带「读图」的模型就能接上。"}
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
        <i className="node" />
        <span className="role">{name}</span>
        {/* Keyed on the assignment so the value replays its entrance when it
            changes — the row is the only thing that moved, and a silent swap
            reads as nothing having happened. */}
        <span className="val" key={set || "follow"}>{set ? (chosen?.model ?? set) : "跟随主模型"}</span>
        <span className="tag">{set ? "已指派" : tag}</span>
      </button>
      {open && (
        <div className="rpick" role="listbox" aria-label={`${name}用哪个模型`}>
          <button role="option" aria-selected={!set} data-cur={!set ? "" : undefined} onClick={() => onPick("")}>
            跟随主模型
          </button>
          <div className="sep" />
          {models.map((m) => (
            <button
              key={m.ref}
              role="option"
              aria-selected={m.ref === set}
              data-cur={m.ref === set ? "" : undefined}
              onClick={() => onPick(m.ref)}
            >
              {m.model}
              <span className="sub">{m.vision ? "读图" : m.provider}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
