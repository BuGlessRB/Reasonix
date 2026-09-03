import { t } from "../../i18n";
import { pct, tokens } from "../../i18n/format";
import type { Metrics } from "../../state/session";
import { useTicker } from "../num";

/** The one figure worth reading first: how much of the prompt the endpoint did
 *  not have to re-read. It leads the rail at full size because a session that
 *  stops hitting its prefix costs several times more per turn, and a number
 *  that size is noticed without being looked for. */
export function Cache({ metrics, done }: { metrics: Metrics; done?: boolean }) {
  const up = metrics.hit + metrics.miss;
  const rate = up ? (metrics.hit / up) * 100 : 0;
  const shown = useTicker(rate);
  const hit = useTicker(metrics.hit);
  const miss = useTicker(metrics.miss);

  return (
    <div className="block" data-b="cache">
      <h3 className="lbl">
        {t("前缀缓存")}
        <span className="c">{up ? t("本会话") : t("还没有请求")}</span>
      </h3>
      <div className="big">
        <span className="v" data-live={up ? "" : undefined} data-flash={done && up ? "" : undefined}>
          {up ? pct(shown / 100, 1) : "—"}
        </span>
        <span className="k">
          {t("前缀命中")}
          <br />
          {t("越高越省")}
        </span>
        {/* 前缀没变是命中率能保住的前提；变了就说变在哪儿。 */}
        {up > 0 && (
          <span
            className="pill"
            data-tone={metrics.prefixChanged ? "warn" : "ok"}
            title={metrics.prefixReasons.join(" · ") || undefined}
          >
            {metrics.prefixChanged ? t("前缀变了") : t("前缀未变")}
          </span>
        )}
        {/* 前缀哈希看不见对话正文，所以"没变"单独一个说明不了未命中是谁的。 */}
        {up > 0 && !!metrics.carriedMessages && (
          <span
            className="pill"
            data-tone={metrics.bodyChanged ? "warn" : "ok"}
            title={t("沿用的消息条数") + ": " + metrics.carriedMessages}
          >
            {metrics.bodyChanged ? t("正文变了") : t("正文未变")}
          </span>
        )}
      </div>
      <div className="bar">
        <i className="c" style={{ flexGrow: Math.max(shown, 0.4) }} />
        <i className="f" style={{ flexGrow: Math.max(100 - shown, 0.4) }} />
      </div>
      <div className="nums">
        <span>{t("命中")}<b>{tokens(Math.round(hit))}</b></span>
        <span>{t("未命中")}<b>{tokens(Math.round(miss))}</b></span>
        {!!metrics.toolSchema && <span>{t("工具 schema")}<b>{tokens(metrics.toolSchema)}</b></span>}
      </div>
      {!!metrics.prefixHash && (
        <div className="nums" style={{ justifyContent: "space-between" }}>
          <span title={metrics.prefixHash}>
            {t("前缀哈希")}<b>{metrics.prefixHash.slice(0, 4)}…{metrics.prefixHash.slice(-4)}</b>
          </span>
        </div>
      )}
    </div>
  );
}
