import type { Guardian } from "../../port/wire";
import { Sym } from "../Sym";

export function GuardianCard({ g }: { g: Guardian }) {
  return (
    <div className="call">
      <div className="g">
        <Sym glyph="⊛" />
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className="nm">Guardian</span>
          <span className="tag">guardian_assessment</span>
          <span className="arg">{g.subject}</span>
        </div>
        <div className="out">
          <div className="guard" data-risk={g.risk_level ?? "low"}>
            <div className="guard-hd">
              <span className="verdict">{g.outcome}</span>
              <span className="gauge" title={`风险 ${g.risk_level ?? ""}`}>
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
