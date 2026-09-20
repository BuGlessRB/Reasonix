import { Fragment, useState } from "react";
import { decimals } from "../i18n/format";
import { t } from "../i18n";
import { categoryOf } from "./icons";
import type { Span, TrajRow } from "../state/trajectory";
import type { TrajectoryAvailability } from "../port/wire";

// The table renders spans; a file wants the text they carry. Both halves of a
// span are values, so this reads them rather than knowing which kinds exist.
const flat = (spans: Span[]) => spans.map((x) => ("b" in x ? x.b : "n" in x ? x.n : x.t)).join("");

// What a row is, without the drawing: when it started, how long it ran, what
// ran. Anything reading this back — a script, a spreadsheet, an issue — wants
// those four, and the rendered payload as the human-readable line.
function serialise(rows: TrajRow[], availability: TrajectoryAvailability | undefined) {
  return JSON.stringify(
    {
      exported: new Date().toISOString(),
      // The file outlives the window that made it, so it carries what the rows
      // cover. A prefix handed over without that reads as a whole session.
      availability,
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

// The same line as one string, for a row narrow enough to clip it: an error
// nobody can read the end of is the half that says what actually failed.
export function spanText(of: Span[]): string {
  return of.map((s) => ("b" in s ? s.b : "n" in s ? s.n : s.t)).join("");
}

export function Spans({ of }: { of: Span[] }) {
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
  const at = (v: number) => `+${decimals(v, 2)}s`;
  const label = dur > 0 ? `${at(start)} → ${at(start + dur)} · ${decimals(dur, 2)}s` : at(start);
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

export function Trajectory({
  rows,
  availability,
  onSave,
}: {
  rows: TrajRow[];
  availability?: TrajectoryAvailability;
  onSave: (name: string, content: string) => Promise<string | null>;
}) {
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
            void onSave(name, serialise(rows, availability)).then((to) => setSaved({ to: to ?? name, path: to !== null }));
          }}
        >
          {t("导出")}
        </button>
        {saved && (
          <span className="traj-saved">
            {saved.path ? t("已保存至 {path}", { path: saved.to }) : t("已下载 {name}", { name: saved.to })}
          </span>
        )}
      </div>
      <table className="traj">
        <thead>
          <tr>
            <th className="seq">seq</th>
            <th className="t">+t</th>
            <th className="tl">
              {t("时间轴")} <span className="tl-span">0 – {decimals(span, 1)}s</span>
            </th>
            <th className="kind">record</th>
            <th>payload</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.seq} data-k={r.kind}>
              <td className="seq">{r.seq}</td>
              <td className="t">{decimals(r.at, 2)}s</td>
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
      {/* What the rows cover, in the host's words. The table cannot tell a
          session that did little from one whose record stops early, and reading
          the last row as the end is exactly the mistake. */}
      <div className="traj-note" data-coverage={availability ?? "unread"}>
        {availability === "truncated"
          ? t("记录已达容量上限而停止 —— 最后一行之后的内容未被保留")
          : availability === "not_recorded"
            ? t("本次运行未记录轨迹 —— 以下仅为本次连接观察到的实时事件")
            : availability === "complete"
              ? t("完整记录 · 重新进入会话将原样重建")
              : t("尚未读取该轨迹的覆盖范围")}
      </div>
    </>
  );
}
