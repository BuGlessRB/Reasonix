import type { Metrics } from "../../state/session";
import { useTicker } from "../num";

const SRC: Record<string, string> = {
  executor: "主循环",
  subagent: "子代理",
  compaction: "压缩",
  planner: "规划",
  classifier: "分类",
  title: "标题",
};

const fmt = (n: number) => n.toLocaleString("en-US");

export function Cache({ metrics, rate: tps, done }: { metrics: Metrics; rate: number; done?: boolean }) {
  const up = metrics.hit + metrics.miss;
  const rate = up ? (metrics.hit / up) * 100 : 0;
  const sources = Object.entries(metrics.bySource).filter(([, v]) => v > 0);
  // The rail's numbers are the same story the run strip tells, so they move the
  // same way: eased to the round's new figure rather than cutting to it.
  const shown = useTicker(rate);
  const hit = useTicker(metrics.hit);
  const miss = useTicker(metrics.miss);
  const out = useTicker(metrics.out);

  return (
    <div className="block" data-b="cache">
      <div className="lbl">缓存</div>
      <div className="big">
        <span className="v" data-live={up ? "" : undefined} data-flash={done && up ? "" : undefined}>
          {shown.toFixed(1)}%
        </span>
        <span className="k">
          前缀命中
          <br />
          本工作区
        </span>
      </div>
      <div className="bar">
        <i className="c" style={{ flexGrow: Math.max(shown, 0.4) }} />
        <i className="f" style={{ flexGrow: Math.max(100 - shown, 0.4) }} />
      </div>
      <div className="legend">
        <div className="r">
          <i style={{ background: "var(--ok)" }} />
          命中<span className="n">{fmt(Math.round(hit))}</span>
        </div>
        <div className="r">
          <i style={{ background: "var(--ghost)" }} />
          未命中<span className="n">{fmt(Math.round(miss))}</span>
        </div>
        <div className="r aside">
          <i />
          输出<span className="n">{fmt(Math.round(out))}</span>
        </div>
      </div>
      <div className="rate" data-live={tps >= 1 ? "" : undefined}>
        <span>下行</span>
        <span className="v">{tps >= 1 ? Math.round(tps) : "—"}</span>
        <span className="u">tok/s</span>
      </div>
      <div className="money">
        <div className="col">
          <span className="k">本工作区</span>
          <span className="v">
            {metrics.currency}
            {metrics.cost.toFixed(2)}
          </span>
          <span className="note">{up ? `命中 ${rate.toFixed(1)}%` : "—"}</span>
        </div>
        <div className="col">
          <span className="k">若不命中</span>
          <span className="v sm">—</span>
          <span className="note">价目未上报</span>
        </div>
      </div>
      <div className="srcs">
        {sources.map(([k, v]) => (
          <div className="r" key={k}>
            <span>{SRC[k] ?? k}</span>
            <span className="n">
              {metrics.currency}
              {v.toFixed(2)}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
