import { Fragment, useState } from "react";
import { t } from "../i18n";
import { categoryOf } from "./icons";
import type { Span, TrajRow } from "../state/trajectory";

// The table renders spans; a file wants the text they carry. Both halves of a
// span are values, so this reads them rather than knowing which kinds exist.
const flat = (spans: Span[]) => spans.map((x) => ("b" in x ? x.b : "n" in x ? x.n : x.t)).join("");

// What a row is, without the drawing: when it started, how long it ran, what
// ran. Anything reading this back — a script, a spreadsheet, an issue — wants
// those four, and the rendered payload as the human-readable line.
function serialise(rows: TrajRow[]) {
  return JSON.stringify(
    {
      exported: new Date().toISOString(),
      span: rows.length ? Number(Math.max(...rows.map((r) => r.at + (r.dur ?? 0))).toFixed(3)) : 0,
      rows: rows.map((r) => ({
        seq: r.seq,
        at: Number(r.at.toFixed(3)),
        dur: r.dur === undefined ? undefined : Number(r.dur.toFixed(3)),
        kind: r.kind,
        tool: r.tool,
        text: flat(r.payload),
        detail: r.subs.map(flat),
      })),
    },
    null,
    2,
  );
}

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

// 一行是一段活动，at 是它开始的时刻 —— 条从那里画，长度就是它跑了多久。
// 并排看下来，重叠的就是并行跑的那几个：一个子代理的长条会罩住它内部的调用。
function Track({ row, span }: { row: TrajRow; span: number }) {
  const dur = row.dur ?? 0;
  const start = row.at;
  // The round is the trunk of a turn, not one more tool; it gets its own tone
  // so the coloured marks read as what happened inside it.
  const cat = row.kind === "model_round" ? "round" : row.tool ? categoryOf(row.tool) : "sys";
  const label = dur > 0 ? `+${start.toFixed(2)}s → +${(start + dur).toFixed(2)}s · ${dur.toFixed(2)}s` : `+${start.toFixed(2)}s`;
  return (
    <span className="tl-track" title={label}>
      <i
        className={dur > 0 ? "tl-bar" : "tl-tick"}
        data-c={cat}
        style={{ left: `${(start / span) * 100}%`, width: dur > 0 ? `${(dur / span) * 100}%` : undefined }}
      />
    </span>
  );
}

export function Trajectory({ rows, onSave }: { rows: TrajRow[]; onSave: (name: string, content: string) => Promise<string | null> }) {
  // 壳能给出落盘路径，浏览器只能说它交给了下载 —— 两句话不一样，别混着说。
  const [saved, setSaved] = useState<{ to: string; path: boolean } | null>(null);
  // 轴的跨度是最后一段活动结束的时刻 —— 不是最后一行开始的时刻，一行现在
  // 代表一段有长度的活动，最长的那条未必是最后开的。
  const span = Math.max(1, ...rows.map((r) => r.at + (r.dur ?? 0)));
  return (
    <>
      <div className="traj-bar">
        <button
          className="btn sm"
          disabled={rows.length === 0}
          onClick={() => {
            const name = `trajectory-${new Date().toISOString().slice(0, 19).replace("T", "-").replace(/:/g, "")}.json`;
            void onSave(name, serialise(rows)).then((to) => setSaved({ to: to ?? name, path: to !== null }));
          }}
        >
          {t("导出")}
        </button>
        {saved && (
          <span className="traj-saved">
            {saved.path ? t("存到 {path}", { path: saved.to }) : t("已下载 {name}", { name: saved.to })}
          </span>
        )}
      </div>
      <table className="traj">
        <thead>
          <tr>
            <th className="seq">seq</th>
            <th className="t">+t</th>
            <th className="tl">
              {t("时间轴")} <span className="tl-span">0 – {span.toFixed(1)}s</span>
            </th>
            <th className="kind">record</th>
            <th>payload</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.seq} data-k={r.kind}>
              <td className="seq">{r.seq}</td>
              <td className="t">{r.at.toFixed(2)}s</td>
              <td className="tl">
                <Track row={r} span={span} />
              </td>
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
      {/* serve builds no trajectory.Recorder — only the CLI does — so there is
          nothing on disk to reload. Say so rather than imply otherwise. */}
      <div className="traj-note">{t("实时事件流 · 仅本次连接，切换或重进会话后重建")}</div>
    </>
  );
}
