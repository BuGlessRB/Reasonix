import { useMemo, useState } from "react";
import { seconds } from "../i18n/format";
import { t } from "../i18n";
import type { GraphNode } from "../port/wire";
import type { GraphState, Step } from "../state/graph";
import { stepsOf } from "../state/graph";
import type { Item } from "../state/session";
import { splitPath } from "./args";
import { callsIn, filesByNode, mergeTouched, spanOf, stateWord, type Touched } from "./gnode";

const pad = (n: number) => String(n).padStart(2, "0");

// Where a step sits on the run's own clock, not the reader's: the offset is
// measured from the earliest node the kernel started, so reopening a transcript
// shows the same times it showed while it ran.
const offset = (ms: number) => {
  const s = Math.max(0, Math.round(ms / 1000));
  return s < 3600 ? `${pad(Math.floor(s / 60))}:${pad(s % 60)}` : `${Math.floor(s / 3600)}:${pad(Math.floor(s / 60) % 60)}:${pad(s % 60)}`;
};

const dur = (ms: number) => seconds(ms, ms < 10_000 ? 1 : 0);

type Calls = Map<string, number | undefined>;

// A step reports, it does not enumerate — the same cap the rail's own panels
// take. The count on the last chip stays honest about the rest.
const CHIPS = 4;

function Files({ files }: { files: Touched[] }) {
  if (files.length === 0) return null;
  return (
    <ul className="tl-files">
      {files.slice(0, CHIPS).map((f) => (
        <li key={f.path} className="tl-file" title={f.path}>
          <span className="nm">{splitPath(f.path)[1]}</span>
          {f.added > 0 && <b className="add">+{f.added}</b>}
          {f.removed > 0 && <b className="del">−{f.removed}</b>}
        </li>
      ))}
      {files.length > CHIPS && <li className="tl-file more">+{files.length - CHIPS}</li>}
    </ul>
  );
}

function Member({ node, calls, onOpen }: { node: GraphNode; calls: Calls; onOpen: (call: string) => void }) {
  const ms = spanOf(node, calls);
  return (
    <li className="tl-mem" data-s={node.state ?? "pending"}>
      <i className="pip" />
      <span className="nm">{node.label || node.id}</span>
      <span className="st">{stateWord(node.state ?? "pending")}</span>
      <span className="rt">{ms != null ? dur(ms) : ""}</span>
      {calls.has(node.id) && (
        <button className="lk" type="button" onClick={() => onOpen(node.id)}>
          {t("定位")}
        </button>
      )}
    </li>
  );
}

function Row({ step, t0, calls, touched, open, onToggle, onOpen }: {
  step: Step;
  t0: number;
  calls: Calls;
  touched: Map<string, Touched[]>;
  open: boolean;
  onToggle: () => void;
  onOpen: (call: string) => void;
}) {
  const { node, members, blocked } = step;
  // A fan-out wrote whatever its workers wrote: the group itself makes no calls,
  // and a file listed once per worker that touched it is the same file three
  // times over.
  const files = mergeTouched([touched.get(node.id), ...members.map((m) => touched.get(m.id))]);
  const state = node.state ?? "pending";
  const ms = spanOf(node, calls);
  // One dependency is worth naming; three do not fit on the line, and a name
  // clipped mid-word hides that there were others at all.
  const sub = blocked.length
    ? blocked.length === 1
      ? t("等 {who}", { who: blocked[0] })
      : t("等 {n} 项上游", { n: blocked.length })
    : members.length
      ? t("{n} 个子代理", { n: members.length })
      : [node.profile, node.model, ms != null ? dur(ms) : ""].filter(Boolean).join(" · ");

  const facts: [string, string | undefined][] = [
    [t("画像"), node.profile],
    [t("模型"), [node.model, node.effort].filter(Boolean).join(" · ")],
    [t("权限"), node.grant === "write" ? t("可写") : node.grant === "read" ? t("只读") : undefined],
    [t("耗时"), ms != null ? dur(ms) : undefined],
    [t("记录"), node.ref],
    [t("错误"), node.err],
  ].filter(([, v]) => v) as [string, string][];

  return (
    <li className="tl-step" data-s={state} data-open={open ? "" : undefined}>
      <i className="tl-dot" aria-hidden="true" />
      <button className="tl-hd" type="button" aria-expanded={open} onClick={onToggle} title={node.id}>
        <span className="tl-top">
          <b className="tl-nm">{node.label || node.id}</b>
          {t0 > 0 && node.startedAt ? <span className="tl-at">{offset(node.startedAt - t0)}</span> : null}
          <span className="tl-badge">{stateWord(state)}</span>
          <i className="tl-chev" aria-hidden="true" />
        </span>
        {sub && <span className="tl-sub">{sub}</span>}
      </button>
      <Files files={files} />
      {open && (
        <div className="tl-body">
          {facts.length > 0 && (
            <dl className="tl-facts">
              {facts.map(([k, v]) => (
                <div key={k}>
                  <dt>{k}</dt>
                  <dd>{v}</dd>
                </div>
              ))}
            </dl>
          )}
          {members.length > 0 && (
            <ul className="tl-mems">
              {members.map((m) => (
                <Member key={m.id} node={m} calls={calls} onOpen={onOpen} />
              ))}
            </ul>
          )}
          {/* Only a node this transcript actually holds can be shown in it: an
              adopted answer was produced by a run that is not in this one. */}
          {calls.has(node.id) && (
            <button className="btn sm" type="button" onClick={() => onOpen(node.id)}>
              {t("在活动流中定位")}
            </button>
          )}
        </div>
      )}
    </li>
  );
}

/** The run as a list, in the order it happened. The activity stream is what was
 *  said and the trajectory is a machine record; neither answers "who did what,
 *  then who" — which for a run made of delegates is the question. */
export function Timeline({ graph, items, onOpen }: { graph: GraphState; items: Item[]; onOpen: (call: string) => void }) {
  const steps = useMemo(() => stepsOf(graph), [graph]);
  const calls = useMemo(() => callsIn(items), [items]);
  const touched = useMemo(() => filesByNode(items), [items]);
  const t0 = useMemo(() => {
    let min = 0;
    for (const n of graph.nodes) if (n.startedAt && (!min || n.startedAt < min)) min = n.startedAt;
    return min;
  }, [graph]);
  // What the reader decided, over what the run is doing. The step in flight is
  // open by default and folds once it settles — unless someone said otherwise
  // about that step, which then holds.
  const [flip, setFlip] = useState<Record<string, boolean>>({});

  if (steps.length === 0) return null;

  return (
    <ol className="tl">
      {steps.map((step) => (
        <Row
          key={step.node.id}
          step={step}
          t0={t0}
          calls={calls}
          touched={touched}
          open={flip[step.node.id] ?? step.node.state === "running"}
          onToggle={() =>
            setFlip((was) => ({ ...was, [step.node.id]: !(was[step.node.id] ?? step.node.state === "running") }))
          }
          onOpen={onOpen}
        />
      ))}
    </ol>
  );
}
