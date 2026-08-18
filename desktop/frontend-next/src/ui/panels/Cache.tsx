import type { Metrics } from "../../state/session";
import { decimals, money, pct } from "../../i18n/format";
import { useTicker, useTrail } from "../num";
import { Spark } from "../Spark";
import { t } from "../../i18n";

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
  const cost = useTicker(metrics.cost);
  // 速度的形状只有攒下来才有 —— 这一栏里唯一每秒都在变的数
  const trail = useTrail(tps, !done);

  return (
    <div className="block" data-b="cache">
      <div className="lbl">{t("缓存")}</div>
      <div className="big">
        <span className="v" data-live={up ? "" : undefined} data-flash={done && up ? "" : undefined}>
          {pct(shown / 100, 1)}
        </span>
        <span className="k">
          {t("前缀命中")}
          <br />
          {t("本会话")}
        </span>
      </div>
      <div className="bar">
        <i className="c" style={{ flexGrow: Math.max(shown, 0.4) }} />
        <i className="f" style={{ flexGrow: Math.max(100 - shown, 0.4) }} />
      </div>
      <div className="legend">
        <div className="r">
          <i style={{ background: "var(--ok)" }} />
          {t("命中")}<span className="n">{fmt(Math.round(hit))}</span>
        </div>
        <div className="r">
          <i style={{ background: "var(--ghost)" }} />
          {t("未命中")}<span className="n">{fmt(Math.round(miss))}</span>
        </div>
        <div className="r aside">
          <i />
          {t("输出")}<span className="n">{fmt(Math.round(out))}</span>
        </div>
      </div>
      <div className="rate" data-live={tps >= 1 ? "" : undefined}>
        <span>{t("下行")}</span>
        <Spark points={trail} />
        <span className="v">{tps >= 1 ? Math.round(tps) : "—"}</span>
        <span className="u">tok/s</span>
      </div>
      <div className="money">
        <div className="col">
          <span className="k">{t("本会话")}</span>
          <span className="v">{money(cost, metrics.currency)}</span>
          <span className="note">{up ? t("命中 {rate}%", { rate: decimals(rate, 1) }) : "—"}</span>
        </div>
        <div className="col">
          <span className="k">{t("若不命中")}</span>
          <span className="v sm">—</span>
          <span className="note">{t("价目未上报")}</span>
        </div>
      </div>
      <div className="srcs">
        {sources.map(([k, v]) => (
          <div className="r" key={k}>
            <span>{t(SRC[k] ?? k)}</span>
            <span className="n">{money(v, metrics.currency)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
