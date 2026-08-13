import type { Metrics } from "../state/session";
import { RMark } from "./RMark";

const kk = (n: number) => (n >= 1000 ? (n / 1000).toFixed(1) + "k" : String(n));

export function RunStrip({ doing, metrics }: { doing: string; metrics: Metrics }) {
  const up = metrics.hit + metrics.miss;
  const rate = up ? ((metrics.hit / up) * 100).toFixed(1) : "0.0";
  return (
    <div className="runstrip">
      <RMark />
      <span className="doing">{doing}</span>
      <span className="io">
        <span className="up">↑ {kk(up)}</span>
        <span className="dn">↓ {kk(metrics.out)}</span>
      </span>
      <span className="mt">
        缓存 <span className="hit">{rate}%</span>
      </span>
      <span className="yen">
        {metrics.currency}
        {metrics.cost.toFixed(2)}
      </span>
    </div>
  );
}
