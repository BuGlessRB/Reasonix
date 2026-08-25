import { useMemo, useState } from "react";
import { seconds } from "../i18n/format";
import { t } from "../i18n";
import type { Item } from "../state/session";
import type { GraphState, Lane } from "../state/graph";
import { lanesOf } from "../state/graph";
import type { GraphNode } from "../port/wire";

// Node boxes are a fixed size, so a lane's geometry follows from its ranks with
// nothing measured. That is what lets the edge layer and the cards agree: both
// read the same numbers instead of one of them reading the DOM after paint.
const W = 190;
const H = 58;
const COLGAP = 64;
const ROWGAP = 12;

const at = (rank: number, row: number) => ({
  x: rank * (W + COLGAP),
  y: row * (H + ROWGAP),
});

interface Placed {
  node: GraphNode;
  x: number;
  y: number;
}

// One link between two nodes, folded from the two facts the kernel publishes
// about them. Ordering and delivery share a line because they share a pair;
// drawing them as two lines would say there are two relationships.
interface Link {
  from: Placed;
  to: Placed;
  carried: boolean;
}

// One key per ordered pair. A node id may hold a slash or a space, so joining
// two of them with either is ambiguous; encoding the pair is not.
const pairKey = (from: string, to: string) => JSON.stringify([from, to]);

function place(lane: Lane): {
  placed: Placed[];
  links: Link[];
  w: number;
  h: number;
} {
  const placed: Placed[] = [];
  lane.ranks.forEach((rank, i) => rank.forEach((node, row) => placed.push({ node, ...at(i, row) })));
  const by = new Map(placed.map((p) => [p.node.id, p]));

  const pairs = new Map<string, { from: Placed; to: Placed; ordered: boolean; carried: boolean }>();
  for (const e of lane.edges) {
    const from = by.get(e.from);
    const to = by.get(e.to);
    if (!from || !to) continue;
    const key = pairKey(e.from, e.to);
    const held = pairs.get(key) ?? { from, to, ordered: false, carried: false };
    if (e.kind === "depends") held.ordered = true;
    if (e.kind === "context") held.carried = true;
    pairs.set(key, held);
  }

  const rows = Math.max(1, ...lane.ranks.map((r) => r.length));
  return {
    placed,
    links: [...pairs.values()].map(({ from, to, carried }) => ({
      from,
      to,
      carried,
    })),
    w: lane.ranks.length * W + Math.max(0, lane.ranks.length - 1) * COLGAP,
    h: rows * H + Math.max(0, rows - 1) * ROWGAP,
  };
}

function path(link: Link): string {
  const x1 = link.from.x + W;
  const y1 = link.from.y + H / 2;
  const x2 = link.to.x;
  const y2 = link.to.y + H / 2;
  const bend = Math.max(22, (x2 - x1) / 2);
  return `M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`;
}

// The calls this transcript holds, by the id a node names. Timing lives on the
// tool stream rather than being published a second time — the node is the call,
// so one id answers both — and membership is what says a node can be shown in
// the activity stream at all: an adopted answer came from a run that is not in
// this transcript.
function callsIn(items: Item[]): Map<string, number | undefined> {
  const out = new Map<string, number | undefined>();
  for (const i of items) {
    if (i.t !== "tool") continue;
    for (const call of [i.tool, ...i.children]) if (call.id) out.set(call.id, call.durationMs ?? undefined);
  }
  return out;
}

// Spelled as literals rather than looked up in a table: the catalogue gate
// reads what sits inside a t() call, and a word laundered through a lookup is
// one it cannot see is missing.
function stateWord(state: string): string {
  switch (state) {
    case "running":
      return t("运行中");
    case "completed":
      return t("完成");
    case "adopted":
      return t("复用");
    case "failed":
      return t("失败");
    case "cancelled":
      return t("已取消");
    case "skipped":
      return t("跳过");
    default:
      return t("待运行");
  }
}

function Node({ p, lane, ms, on, chosen }: { p: Placed; lane: Lane; ms?: number; on: () => void; chosen: boolean }) {
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
  const { placed, links, w, h } = useMemo(() => place(lane), [lane]);
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
            {links.map((l) => (
              <path
                key={pairKey(l.from.node.id, l.to.node.id)}
                className="gl-edge"
                data-carried={l.carried ? "" : undefined}
                d={path(l)}
                markerEnd={`url(#${l.carried ? "gm-full" : "gm-open"})`}
              />
            ))}
          </svg>
          {placed.map((p) => (
            <Node
              key={p.node.id}
              p={p}
              lane={lane}
              ms={calls.get(p.node.id)}
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
  const rows: [string, string | undefined][] = [
    [t("状态"), stateWord(node.state ?? "pending")],
    [t("画像"), node.profile],
    [t("模型"), [node.model, node.effort].filter(Boolean).join(" · ")],
    [t("权限"), node.grant === "write" ? t("可写") : node.grant === "read" ? t("只读") : undefined],
    [t("耗时"), ms != null ? seconds(ms, 2) : undefined],
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
          "这一轮没有派出并行的子代理。fleet 或 parallel_tasks 一开跑，它们的形状就画在这里 —— 谁在等谁，谁在同时跑，哪些答案是复用的。",
        )}
      </p>
    );
  }

  const ran = lanes.reduce((n, l) => n + l.ran, 0);
  const reused = lanes.reduce((n, l) => n + l.reused, 0);
  const waiting = lanes.reduce((n, l) => n + Object.keys(l.blocked).length, 0);

  return (
    <>
      <div className="gsum">
        <span className="gb-n">
          {t("{n} 个子代理", { n: ran + reused })}
          {reused > 0 && <em>{t("· {n} 个没有重跑", { n: reused })}</em>}
          {waiting > 0 && <em className="on">{t("· {n} 个在等上游", { n: waiting })}</em>}
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
          ms={calls.get(picked.node.id)}
          // Only a node this transcript actually holds can be shown in it: an
          // adopted answer was produced by a run that is not in this one.
          onOpen={calls.has(picked.node.id) ? () => onOpen(picked.node.id) : undefined}
        />
      )}
    </>
  );
}
