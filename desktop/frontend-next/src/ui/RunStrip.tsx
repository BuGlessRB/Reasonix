import type { Metrics } from "../state/session";
import { RMark } from "./RMark";
import { useBump, useFresh, useTicker } from "./num";

const kk = (n: number) => (n >= 1000 ? (n / 1000).toFixed(1) + "k" : String(Math.round(n)));

const clock = (secs: number) => {
  const s = Math.floor(secs);
  return s < 60 ? `${s} 秒` : `${Math.floor(s / 60)} 分 ${s % 60} 秒`;
};

interface Props {
  doing: string;
  metrics: Metrics;
  steps: number;
  elapsed: number;
}

export function RunStrip({ doing, metrics, steps, elapsed }: Props) {
  const up = metrics.hit + metrics.miss;
  const rate = up ? (metrics.hit / up) * 100 : 0;
  // Usage arrives per model round, not per token: the counters are honest only
  // at those boundaries. Easing between them shows movement without claiming a
  // resolution the wire does not have.
  const upShown = useTicker(up);
  const outShown = useTicker(metrics.out);
  const rateShown = useTicker(rate);
  const costShown = useTicker(metrics.cost);
  return (
    <div className="runstrip" data-rx={useFresh(metrics.out) ? "" : undefined}>
      <RMark />
      <span className="doing">{doing}</span>
      <span className="mt">{steps ? `${steps} 步 · ${clock(elapsed)}` : ""}</span>
      <span className="io">
        <span className="up" data-bump={useBump(up) ? "" : undefined}>
          ↑ {kk(upShown)}
        </span>
        <span className="dn" data-bump={useBump(metrics.out) ? "" : undefined}>
          ↓ {kk(outShown)}
        </span>
      </span>
      <span className="mt">
        缓存 <span className="hit">{rateShown.toFixed(1)}%</span>
      </span>
      <span className="yen">
        {metrics.currency}
        {costShown.toFixed(2)}
      </span>
    </div>
  );
}
