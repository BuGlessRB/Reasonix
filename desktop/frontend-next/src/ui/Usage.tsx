import { useEffect, useMemo, useState } from "react";
import { current, t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AgentPort, Money, UsageDay, UsageReport } from "../port/port";

const RANGES: [number, string][] = [[7, "7 天"], [30, "30 天"], [365, "全部"]];

// Tokens run to nine figures; a raw group-separated number stops being a
// quantity a reader can hold. The break points are the language's, not a
// translation of one set of them: Chinese counts in 万/亿, English in K/M/B.
function short(n: number): string {
  if (current() === "zh") {
    if (n >= 1e8) return trim((n / 1e8).toFixed(2)) + " 亿";
    if (n >= 1e4) return trim((n / 1e4).toFixed(1)) + " 万";
    return n.toLocaleString();
  }
  if (n >= 1e9) return trim((n / 1e9).toFixed(2)) + "B";
  if (n >= 1e6) return trim((n / 1e6).toFixed(1)) + "M";
  if (n >= 1e3) return trim((n / 1e3).toFixed(1)) + "K";
  return n.toLocaleString();
}
const trim = (v: string) => v.replace(/\.0+$/, "");

const SYMBOL: Record<string, string> = { CNY: "¥", USD: "$" };
function money(m: Money): string {
  return (SYMBOL[m.currency] ?? m.currency + " ") + trim(Number(m.amount).toFixed(2));
}

// Two currencies are shown side by side. Adding them would need an exchange
// rate nobody recorded, and the rate a turn was billed at is not today's.
function moneyList(list?: Money[]): string {
  if (!list?.length) return "—";
  return list.map(money).join("  ·  ");
}

// A tick ladder a person reads: round the top up to a 1/2/5-ish step.
function niceMax(raw: number): number {
  if (raw <= 0) return 1;
  const pow = Math.pow(10, Math.floor(Math.log10(raw)));
  return [1, 1.2, 1.5, 2, 2.5, 3, 4, 5, 6, 8, 10].map((m) => m * pow).find((v) => v >= raw) ?? raw;
}

interface PlotProps {
  days: UsageDay[];
  pick: (d: UsageDay) => number;
  label: (v: number) => string;
  aria: string;
}

// One series, so no legend: the card's title names it. Area fill, recessive
// grid, an emphasised endpoint, and a crosshair that reads the nearest day.
function Plot({ days, pick, label, aria }: PlotProps) {
  const [at, setAt] = useState<number | null>(null);
  const W = 860, H = 176, PL = 58, PR = 12, PT = 12, PB = 24;
  const max = niceMax(Math.max(...days.map(pick), 0));
  const x = (i: number) => PL + (days.length < 2 ? 0 : (i * (W - PL - PR)) / (days.length - 1));
  const y = (v: number) => PT + (1 - v / max) * (H - PT - PB);
  const pts = days.map((d, i) => [x(i), y(pick(d))] as const);
  const line = pts.map((p, i) => (i ? "L" : "M") + p[0].toFixed(1) + " " + p[1].toFixed(1)).join(" ");
  const fill = pts.length ? `${line} L${x(days.length - 1).toFixed(1)} ${H - PB} L${PL} ${H - PB} Z` : "";
  const every = Math.max(1, Math.ceil(days.length / 5));
  const hover = at === null ? null : days[at];

  return (
    <div className="uplot" data-on={hover ? "" : undefined}>
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={aria}>
        <g className="ugrid">
          {[0, max / 2, max].map((v) => (
            <line key={v} x1={PL} x2={W - PR} y1={y(v)} y2={y(v)} />
          ))}
        </g>
        <g className="uaxis">
          {[0, max / 2, max].map((v) => (
            <text key={v} x={PL - 8} y={y(v) + 4} textAnchor="end">{label(v)}</text>
          ))}
          {days.map((d, i) =>
            i % every ? null : (
              <text key={d.day} x={x(i)} y={H - 7} textAnchor="middle">{d.day.slice(5)}</text>
            ),
          )}
        </g>
        {fill && <path className="uarea" d={fill} />}
        {pts.length > 1 && <path className="uline" d={line} />}
        {pts.length > 0 && <circle className="ucap" cx={pts[pts.length - 1][0]} cy={pts[pts.length - 1][1]} r={4} />}
        {hover && at !== null && (
          <>
            <line className="ucross" x1={x(at)} x2={x(at)} y1={PT} y2={H - PB} />
            <circle className="umark" cx={x(at)} cy={y(pick(hover))} r={4.5} />
          </>
        )}
        <rect
          className="uhit" x={PL} y={0} width={W - PL - PR} height={H}
          onPointerMove={(e) => {
            const box = e.currentTarget.ownerSVGElement!.getBoundingClientRect();
            const vx = ((e.clientX - box.left) / box.width) * W;
            let best = 0, dist = Infinity;
            days.forEach((_, i) => {
              const d = Math.abs(x(i) - vx);
              if (d < dist) { dist = d; best = i; }
            });
            setAt(best);
          }}
          onPointerLeave={() => setAt(null)}
        />
      </svg>
      {hover && at !== null && (
        <div className="utip" style={{ left: `${(x(at) / W) * 100}%`, top: `${(y(pick(hover)) / H) * 100}%` }}>
          <div className="d">{hover.day}</div>
          <div className="v">{label(pick(hover))}</div>
        </div>
      )}
    </div>
  );
}

// Nominal rows all take the same hue — the bar's length already carries the
// value, so spending the colour channel on it says nothing new.
function Bars({ rows }: { rows: [string, number, string][] }) {
  const top = Math.max(...rows.map((r) => r[1]), 1);
  return (
    <div className="urows">
      {rows.map(([name, value, note]) => (
        <div className="urow" key={name}>
          <div className="urow-t">
            <span className="n" title={name}>{name}</span>
            <span className="v">{note}</span>
          </div>
          <div className="utrack"><i className="ufill" style={{ width: `${(value / top) * 100}%` }} /></div>
        </div>
      ))}
    </div>
  );
}

export function Usage({ port }: { port: AgentPort }) {
  const [days, setDays] = useState(30);
  const [report, setReport] = useState<UsageReport | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    let live = true;
    setErr("");
    port.usage(days).then(
      (r) => live && setReport(r),
      (e) => live && setErr(reason(e)),
    );
    return () => { live = false; };
  }, [port, days]);

  // The first day that carries a cost. Records older than the cost field have
  // tokens and no price, and showing them as free would understate the bill.
  const pricedFrom = useMemo(
    () => report?.daily.find((d) => d.cost?.length)?.day ?? "",
    [report],
  );
  const priced = useMemo(
    () => report?.daily.filter((d) => d.cost?.length) ?? [],
    [report],
  );
  const unpricedDays = (report?.daily.filter((d) => d.total > 0 && !d.cost?.length) ?? []).length;
  const hitRate = report && report.cache_hit + report.cache_miss > 0
    ? (report.cache_hit / (report.cache_hit + report.cache_miss)) * 100
    : 0;

  if (err) return <div className="uerr">{err}</div>;
  if (!report) return <div className="uwait">{t("正在读记录…")}</div>;

  return (
    <div className="usage">
      <div className="uhead">
        <div className="uranges" role="group" aria-label={t("时间范围")}>
          {RANGES.map(([n, lbl]) => (
            <button key={n} type="button" aria-pressed={days === n} onClick={() => setDays(n)}>{t(lbl)}</button>
          ))}
        </div>
        <span className="uspan">{report.from} → {report.to}</span>
      </div>

      <section className="uhero">
        <div className="uhero-k">{t("实付")}</div>
        <div className="uhero-fig">
          <div className="uhero-v">{moneyList(report.cost)}</div>
          {pricedFrom && <span className="uhero-scope">{t("{day} 起 · {n} 天有记录", { day: pricedFrom.slice(5), n: priced.length })}</span>}
        </div>
        <p className="uhero-note">
          {t("这段时间 {tok} tokens 里有 {rate}% 命中前缀缓存 —— 命中的部分按缓存价计费，比未命中便宜一个量级。", {
            tok: short(report.tokens), rate: hitRate.toFixed(1),
          })}
        </p>
      </section>

      <div className="utiles">
        <div className="utile"><div className="k">Tokens</div><div className="v">{short(report.tokens)}</div>
          <div className="m">{t("命中 {a} · 未命中 {b}", { a: short(report.cache_hit), b: short(report.cache_miss) })}</div></div>
        <div className="utile"><div className="k">{t("缓存命中率")}</div><div className="v">{hitRate.toFixed(1)}%</div>
          <div className="m">{t("越高越省 —— 前缀保持稳定")}</div></div>
        <div className="utile"><div className="k">Turns</div><div className="v">{report.turns.toLocaleString()}</div>
          <div className="m">{t("{n} 次 API 请求", { n: report.requests.toLocaleString() })}</div></div>
        <div className="utile"><div className="k">{t("活跃天数")}</div><div className="v">{report.active_days}</div>
          <div className="m">{t("共 {n} 天中", { n: report.daily.length })}</div></div>
      </div>

      <section className="ucard">
        <div className="ucard-h"><h3>{t("每日用量")}</h3><span className="hint">{t("悬停查看某天")}</span></div>
        <Plot days={report.daily} pick={(d) => d.total} label={(v) => short(Math.round(v))} aria={t("每日 tokens")} />
      </section>

      {priced.length > 0 && (
        <section className="ucard">
          <div className="ucard-h"><h3>{t("每日支出")}</h3><span className="hint">{priced[0].cost?.[0]?.currency}</span></div>
          <p className="ucard-s">{t("与用量分开画：两者量纲不同，共用一根轴会读错")}</p>
          <Plot days={priced} pick={(d) => Number(d.cost?.[0]?.amount ?? 0)}
            label={(v) => (SYMBOL[priced[0].cost?.[0]?.currency ?? ""] ?? "") + v.toFixed(2)}
            aria={t("每日支出")} />
          {unpricedDays > 0 && (
            <div className="unote">
              {t("另有 {n} 天有用量但没有成本记录 —— 不是那几天没花费，是成本字段后来才开始持久化。", { n: unpricedDays })}
            </div>
          )}
        </section>
      )}

      <div className="utwo">
        <section className="ucard">
          <div className="ucard-h"><h3>{t("按模型")}</h3></div>
          <Bars rows={report.models.map((m) => [m.model, m.tokens, `${short(m.tokens)}  ${m.percent.toFixed(1)}%`])} />
        </section>
        <section className="ucard">
          <div className="ucard-h"><h3>{t("按来源")}</h3></div>
          <Bars rows={report.providers.map((p) => [p.provider, p.tokens, `${short(p.tokens)}  ${p.percent.toFixed(1)}%`])} />
        </section>
      </div>
    </div>
  );
}
