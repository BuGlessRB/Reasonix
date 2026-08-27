import { t } from "../../i18n";
import { Spark } from "../Spark";
import { useTrail } from "../num";
import type { Stats } from "./derive";
import { Grp, Row } from "./kit";

interface Props {
  rate: number;
  done: boolean;
  stats: Stats;
  files: number;
}

/** What keeping the run moving is costing. The three headline figures are on
 *  the head card; this is the composition behind them — where the window went,
 *  whether the cache is holding, whether throughput is falling off. Throughput
 *  is the one figure the window observes itself, sample by sample; nothing on
 *  the wire reports a per-request duration, so no row claims one. */
export function Runtime({ rate, done, stats, files }: Props) {
  const trail = useTrail(rate, !done);

  return (
    <Grp name={t("运行详情")}>
      <Row
        k={t("吞吐（输出）")}
        v={
          <span className="rate" data-live={rate >= 1 ? "" : undefined}>
            <Spark points={trail} />
            <span className="v">{rate >= 1 ? Math.round(rate) : "—"}</span>
            <span className="u">tok/s</span>
          </span>
        }
      />
      <Row k={t("工具调用次数")} v={stats.tools} />
      <Row k={t("失败步数")} v={stats.failed} tone={stats.failed ? "err" : undefined} />
      <Row k={t("待确认")} v={stats.waiting} tone={stats.waiting ? "accent" : undefined} />
      <Row k={t("外部请求")} v={stats.external} />
      <Row k={t("改动文件")} v={files} />
    </Grp>
  );
}
