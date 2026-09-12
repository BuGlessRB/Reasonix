import type { Guardian } from "../../port/wire";
import { Sym } from "../Sym";
import { t } from "../../i18n";

export function GuardianCard({ g }: { g: Guardian }) {
  const risk = g.risk_level ?? "unknown";
  const riskLabel = risk === "low" ? t("低风险") : risk === "medium" ? t("中风险") : risk === "high" ? t("高风险") : t("风险未知");
  return (
    <div className="call" data-k="host">
      <div className="g">
        <Sym glyph="⊛" />
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          {/* tag 这一格别的卡放的是 git-bash、security-review 这种人能读的东西。
              事件名属于轨迹，不属于这里。 */}
          <span className="nm">{t("守卫复核")}</span>
          <span className="tag">{t("主机")}</span>
          <span className="arg">{g.subject}</span>
        </div>
        <div className="out">
          <div className="guard" data-risk={risk}>
            <div className="guard-hd">
              <span className="verdict">{g.outcome}</span>
              <span className="risk">{riskLabel}</span>
              <span className="gauge" aria-label={riskLabel}>
                <i />
                <i />
                <i />
              </span>
            </div>
            {g.rationale && <div className="guard-why">{g.rationale}</div>}
          </div>
        </div>
      </div>
    </div>
  );
}
