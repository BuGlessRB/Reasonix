import { t } from "../../i18n";
import { Spark } from "../Spark";
import { money, tokens } from "../../i18n/format";
import type { Metrics } from "../../state/session";
import { Grp, Row } from "./kit";

const SRC: Record<string, string> = {
  executor: "主循环",
  subagent: "子代理",
  compaction: "压缩",
  planner: "规划",
  classifier: "分类",
  title: "标题",
};

export function Cost({ metrics }: { metrics: Metrics }) {
  const sources = Object.entries(metrics.bySource).filter(([, v]) => v > 0);
  const peak = metrics.rounds.length ? Math.max(...metrics.rounds) : 0;
  const avg = metrics.rounds.length ? Math.round(metrics.rounds.reduce((a, b) => a + b, 0) / metrics.rounds.length) : 0;
  return (
    <Grp name={t("成本")} aside={t("本会话")}>
      <div className="mrow">
        <span className="amt">{money(metrics.cost, metrics.currency)}</span>
        {/* 一个按公布价计出来的数和一个拿兑底表估的数是两种断言。 */}
        {sources.length > 0 && (
          <span className="pill" data-tone={metrics.estimated ? "warn" : "ok"}>
            {metrics.estimated ? t("按兜底价估") : t("已结算")}
          </span>
        )}
        {metrics.turn > 0 && <span className="delta">{t("本回合")}+{money(metrics.turn, metrics.currency)}</span>}
        {sources.length === 0 && <span className="msub">{t("价目未上报")}</span>}
      </div>
      {metrics.alt && (
        <div className="mrow" data-alt="">
          <span className="amt">{money(metrics.alt.amount, metrics.alt.currency)}</span>
          <span className="msub">{t("原币种")}</span>
        </div>
      )}
      {metrics.alt && <p className="mnote">{t("两种结算币不相加 —— 合成一个总数就得凭空发明一个汇率。")}</p>}
      {sources.map(([k, v]) => <Row key={k} k={t(SRC[k] ?? k)} v={money(v, metrics.currency)} />)}
      {metrics.rounds.length > 1 && (
        <div className="sparkbox">
          <div className="sphd">
            <span>{t("每回合用量")}</span>
            <span className="m">
              {t("峰值 {peak} · 均 {avg}", { peak: tokens(peak), avg: tokens(avg) })}
            </span>
          </div>
          <Spark points={metrics.rounds} h={30} />
        </div>
      )}
    </Grp>
  );
}
