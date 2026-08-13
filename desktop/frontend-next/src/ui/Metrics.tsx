import type { Metrics as M, PlanStep } from "../state/session";
import { Plan } from "./Plan";

const SRC: Record<string, string> = {
  executor: "主循环",
  subagent: "子代理",
  compaction: "压缩",
  planner: "规划",
  classifier: "分类",
  title: "标题",
};

export function Metrics({ metrics, plan, onFold }: { metrics: M; plan: PlanStep[]; onFold: () => void }) {
  const up = metrics.hit + metrics.miss;
  const rate = up ? (metrics.hit / up) * 100 : 0;
  const sources = Object.entries(metrics.bySource).filter(([, v]) => v > 0);

  return (
    <>
      <div className="side-hd">
        <div className="lbl">度量</div>
        <button className="collapse" onClick={onFold} aria-label="收起度量栏">
          ›
        </button>
      </div>
      <div className="scroll">
        <div className="block" data-b="cache">
          <div className="lbl">缓存</div>
          <div className="big">
            <span className="v" data-live={up ? "" : undefined}>
              {rate.toFixed(1)}%
            </span>
            <span className="k">
              前缀命中
              <br />
              本工作区
            </span>
          </div>
          <div className="bar">
            <i className="c" style={{ flexGrow: Math.max(rate, 0.4) }} />
            <i className="f" style={{ flexGrow: Math.max(100 - rate, 0.4) }} />
          </div>
          <div className="legend">
            <div className="r">
              <i style={{ background: "var(--ok)" }} />
              命中<span className="n">{metrics.hit.toLocaleString()}</span>
            </div>
            <div className="r">
              <i style={{ background: "var(--ghost)" }} />
              未命中<span className="n">{metrics.miss.toLocaleString()}</span>
            </div>
            <div className="r aside">
              <i />
              输出<span className="n">{metrics.out.toLocaleString()}</span>
            </div>
          </div>
          {sources.length > 0 && (
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
          )}
        </div>
        <Plan steps={plan} />
      </div>
    </>
  );
}
