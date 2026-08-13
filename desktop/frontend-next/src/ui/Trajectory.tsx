import { Fragment } from "react";
import type { Span, TrajRow } from "../state/trajectory";

function Spans({ of }: { of: Span[] }) {
  return (
    <>
      {of.map((s, i) => (
        <Fragment key={i}>
          {"b" in s ? <b>{s.b}</b> : "n" in s ? <span className="num">{s.n}</span> : s.t}
        </Fragment>
      ))}
    </>
  );
}

export function Trajectory({ rows }: { rows: TrajRow[] }) {
  return (
    <>
      <table className="traj">
        <thead>
          <tr>
            <th className="seq">seq</th>
            <th className="t">+t</th>
            <th className="kind">record</th>
            <th>payload</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.seq} data-k={r.kind}>
              <td className="seq">{r.seq}</td>
              <td className="t">{r.at.toFixed(2)}s</td>
              <td className="kind">{r.kind}</td>
              <td>
                <Spans of={r.payload} />
                {r.subs.map((sub, i) => (
                  <span className="sub" key={i}>
                    <Spans of={sub} />
                  </span>
                ))}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="traj-note">
        事件流实录 · 与 <code>internal/trajectory</code> 落盘同源
      </div>
    </>
  );
}
