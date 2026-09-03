import { useMemo, useState } from "react";
import { seconds } from "../i18n/format";
import { t } from "../i18n";
import type { Item } from "../state/session";
import type { GraphState, Lane } from "../state/graph";
import { lanesOf } from "../state/graph";
import type { GraphNode } from "../port/wire";
import { callsIn, slotWaitOf, spanOf, stateWord } from "./gnode";
import { H, W, layout, pairKey, wirePath, type PlacedNode } from "./glayout";

// Geometry, ordering and edge routing live in glayout: a lane's arrangement is
// arithmetic over what the kernel published, and keeping it out of the view is
// what lets it be checked without rendering anything.
function Node({ p, lane, ms, on, chosen }: { p: PlacedNode; lane: Lane; ms?: number; on: () => void; chosen: boolean }) {
  const { node } = p;
  const state = node.state ?? "pending";
  const waiting = lane.blocked[node.id];
  // One dependency is worth naming; three do not fit on a card, and a name
  // clipped mid-word hides that there were others at all.
  const waitingFor = waiting?.length === 1 ? labelOf(lane, waiting[0]) : undefined;
  const reused = lane.reuse[node.id];
  // The state is a word on every card, so the colour repeats it rather than
  // being the only thing that carries it.
  const meta = [node.profile, node.model, ms != null ? seconds(ms, ms < 10_000 ? 1 : 0) : ""].filter(Boolean).join(" · ");
  return (
    <button
      className="gnode"
      type="button"
      data-s={state}
      data-w={node.grant === "write" ? "" : undefined}
      aria-pressed={chosen}
      style={{ left: p.x, top: p.y, width: W, height: H }}
      onClick={on}
      title={node.id}
    >
      <span className="gn-hd">
        <i className="pip" />
        <span className="gn-nm">{node.label || node.id}</span>
        {reused && <b className="gn-re">⟲</b>}
      </span>
      <span className="gn-meta">
        {waiting ? (
          <span className="gn-wait">
            {waitingFor ? t("等 {who}", { who: waitingFor }) : t("等 {n} 项上游", { n: waiting.length })}
          </span>
        ) : (
          <>
            <span className="gn-st">{stateWord(state)}</span>
            {meta && <span className="gn-sub">{meta}</span>}
          </>
        )}
      </span>
    </button>
  );
}

// Names read from the lane rather than ids: "waiting on survey" is the sentence
// a reader can act on, and the id is on the detail strip for when it is not.
function namesOf(lane: Lane, ids: string[]): string {
  return ids.map((id) => labelOf(lane, id)).join("、");
}

function labelOf(lane: Lane, id: string): string {
  for (const rank of lane.ranks) {
    for (const n of rank) if (n.id === id) return n.label || n.id;
  }
  return id;
}

function LaneView({
  lane,
  calls,
  chosen,
  onChoose,
}: {
  lane: Lane;
  calls: Map<string, number | undefined>;
  chosen: string | null;
  onChoose: (id: string) => void;
}) {
  const { nodes, wires, w, h } = useMemo(() => layout(lane), [lane]);
  const state = lane.group.state ?? "running";
  return (
    <section className="glane">
      <header className="gl-hd">
        <i className="pip" data-s={state} />
        <b>{lane.group.label || lane.group.id}</b>
        <span className="gl-st">{stateWord(state)}</span>
        <span className="gl-n">
          {t("{n} 个跑了", { n: lane.ran })}
          {lane.reused > 0 && <em>{t("· {n} 个复用", { n: lane.reused })}</em>}
        </span>
      </header>
      <div className="gl-scroll">
        <div className="gl-plot" style={{ width: w, height: h }}>
          <svg className="gl-wire" width={w} height={h} aria-hidden>
            <defs>
              <marker id="gm-full" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto">
                <path d="M0 0 L8 4 L0 8 z" fill="context-stroke" />
              </marker>
              <marker id="gm-open" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto">
                <path d="M0.5 0.5 L7 4 L0.5 7.5" fill="none" stroke="context-stroke" strokeWidth="1.2" />
              </marker>
            </defs>
            {wires.map((wire) => (
              <path
                key={pairKey(wire.from, wire.to)}
                className="gl-edge"
                data-carried={wire.carried ? "" : undefined}
                d={wirePath(wire.points)}
                markerEnd={`url(#${wire.carried ? "gm-full" : "gm-open"})`}
              />
            ))}
          </svg>
          {nodes.map((p) => (
            <Node
              key={p.node.id}
              p={p}
              lane={lane}
              ms={spanOf(p.node, calls)}
              chosen={chosen === p.node.id}
              on={() => onChoose(p.node.id)}
            />
          ))}
        </div>
      </div>
    </section>
  );
}

function Detail({ node, lane, ms, onOpen }: { node: GraphNode; lane: Lane; ms?: number; onOpen?: () => void }) {
  const waiting = lane.blocked[node.id];
  // Slot wait is time the run spent ready and not running, so it belongs beside
  // the time it spent working: together they say where the node's span went.
  const wait = slotWaitOf(node);
  const rows: [string, string | undefined][] = [
    [t("状态"), stateWord(node.state ?? "pending")],
    [t("画像"), node.profile],
    [t("模型"), [node.model, node.effort].filter(Boolean).join(" · ")],
    [t("权限"), node.grant === "write" ? t("可写") : node.grant === "read" ? t("只读") : undefined],
    [t("耗时"), ms != null ? seconds(ms, 2) : undefined],
    [t("等空位"), wait != null ? seconds(wait, 2) : undefined],
    [t("等待"), waiting && namesOf(lane, waiting)],
    [t("复用自"), lane.reuse[node.id]],
    [t("记录"), node.ref],
    [t("错误"), node.err],
  ];
  return (
    <aside className="gdetail">
      <header>
        {node.label || node.id}
        {onOpen && (
          <button className="btn sm" type="button" onClick={onOpen}>
            {t("在活动流中定位")}
          </button>
        )}
      </header>
      <dl>
        {rows
          .filter(([, v]) => v)
          .map(([k, v]) => (
            <div key={k}>
              <dt>{k}</dt>
              <dd>{v}</dd>
            </div>
          ))}
      </dl>
      <p className="gd-id">{node.id}</p>
    </aside>
  );
}

export function Graph({ graph, items, onOpen }: { graph: GraphState; items: Item[]; onOpen: (call: string) => void }) {
  const [chosen, setChosen] = useState<string | null>(null);
  const lanes = useMemo(() => lanesOf(graph), [graph]);
  const calls = useMemo(() => callsIn(items), [items]);

  const picked = useMemo(() => {
    for (const lane of lanes) {
      for (const rank of lane.ranks) {
        for (const n of rank) if (n.id === chosen) return { node: n, lane };
      }
    }
    return null;
  }, [lanes, chosen]);

  if (lanes.length === 0) {
    return (
      <p className="gempty">
        {t(
          "这一轮还没有派出子代理。派出去了就画在这里 —— 谁在跑、跑成什么样；成组派出时还有谁在等谁、谁在同时跑、哪些答案是复用的。",
        )}
      </p>
    );
  }

  const ran = lanes.reduce((n, l) => n + l.ran, 0);
  const reused = lanes.reduce((n, l) => n + l.reused, 0);
  const waiting = lanes.reduce((n, l) => n + Object.keys(l.blocked).length, 0);
  // Told apart from waiting on purpose: one is answered by finishing upstream
  // work, the other only by raising the session's concurrency ceiling.
  const queued = lanes.reduce((n, l) => n + l.queued, 0);

  return (
    <>
      <div className="gsum">
        <span className="gb-n">
          {t("{n} 个子代理", { n: ran + reused })}
          {reused > 0 && <em>{t("· {n} 个没有重跑", { n: reused })}</em>}
          {waiting > 0 && <em className="on">{t("· {n} 个在等上游", { n: waiting })}</em>}
          {queued > 0 && <em className="on">{t("· {n} 个在等空位", { n: queued })}</em>}
        </span>
        {/* 边有两种,靠线型分,不靠颜色分 —— 排了序和真把答案交过去是两件事。 */}
        <span className="gkey">
          <span className="gk">
            <i className="gk-line" data-carried="" />
            {t("依赖 · 答案已交付")}
          </span>
          <span className="gk">
            <i className="gk-line" />
            {t("只排了序")}
          </span>
          <span className="gk">
            <b className="gk-re">⟲</b>
            {t("复用了已完成的结果")}
          </span>
        </span>
      </div>
      <div className="glanes">
        {lanes.map((lane) => (
          <LaneView
            key={lane.group.id}
            lane={lane}
            calls={calls}
            chosen={chosen}
            onChoose={(id) => setChosen((was) => (was === id ? null : id))}
          />
        ))}
      </div>
      {picked && (
        <Detail
          node={picked.node}
          lane={picked.lane}
          ms={spanOf(picked.node, calls)}
          // Only a node this transcript actually holds can be shown in it: an
          // adopted answer was produced by a run that is not in this one.
          onOpen={calls.has(picked.node.id) ? () => onOpen(picked.node.id) : undefined}
        />
      )}
    </>
  );
}
