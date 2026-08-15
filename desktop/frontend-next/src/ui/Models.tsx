import { useCallback, useMemo, useRef, useState } from "react";
import type { ModelEntry } from "../port/port";
import { useDismiss } from "./dismiss";

// The picker used to be one flat list of provider×model, which is three
// independent axes multiplied out: which service, which protocol reaches it,
// which model. Here the service groups the rows, the protocol folds into a
// route selector, and only the model is a choice you make by clicking.

const KIND_LABEL: Record<string, string> = {
  openai: "OpenAI",
  anthropic: "Anthropic",
  responses: "Responses",
  extension: "扩展",
};

const CURRENCY: Record<string, string> = { CNY: "¥", USD: "$", EUR: "€" };

// A folded row: one model, and every route that reaches it at this endpoint.
interface Folded {
  key: string;
  model: string;
  routes: ModelEntry[];
}

interface Group {
  key: string;
  label: string;
  host: string;
  rows: Folded[];
}

function vendorKey(m: ModelEntry): string {
  return (m.vendor || m.provider || "").toLowerCase();
}

export function groupModels(models: ModelEntry[]): Group[] {
  const groups = new Map<string, Group>();
  const rows = new Map<string, Folded>();
  for (const m of models) {
    const vk = vendorKey(m);
    let g = groups.get(vk);
    if (!g) {
      g = { key: vk, label: "", host: m.vendor ?? "", rows: [] };
      groups.set(vk, g);
    }
    const rk = vk + "\u0000" + m.model;
    let row = rows.get(rk);
    if (!row) {
      row = { key: rk, model: m.model, routes: [] };
      rows.set(rk, row);
      g.rows.push(row);
    }
    row.routes.push(m);
  }
  for (const g of groups.values()) {
    const names: string[] = [];
    for (const row of g.rows) {
      for (const r of row.routes) {
        if (!names.includes(r.provider)) names.push(r.provider);
      }
    }
    g.label = names.join(" · ");
  }
  return [...groups.values()];
}

function contextLabel(tokens?: number): string {
  if (!tokens || tokens <= 0) return "";
  if (tokens >= 1_000_000) {
    const m = tokens / 1_000_000;
    return `${Number.isInteger(m) ? m : m.toFixed(1)}M`;
  }
  return `${Math.round(tokens / 1024)}K`;
}

function priceLabel(m: ModelEntry): string {
  const p = m.price;
  if (!p) return "";
  const sign = CURRENCY[p.currency ?? ""] ?? "";
  const n = (v: number) => (Number.isInteger(v) ? String(v) : v.toFixed(2).replace(/0$/, ""));
  return `${sign}${n(p.input)} / ${sign}${n(p.output)}`;
}

// Every tag needs something in the config or the probe behind it. An inferred
// "reads images" badge is worse than a blank row: it sends the user to a
// request the endpoint rejects, with nothing on screen explaining why.
function tagsFor(m: ModelEntry): [string, string][] {
  const out: [string, string][] = [];
  if (m.vision) out.push(["vis", "读图"]);
  if (m.efforts && m.efforts.length > 1) out.push(["think", "推理"]);
  const ctx = contextLabel(m.contextWindow);
  if (ctx) out.push(["ctx", ctx]);
  const price = priceLabel(m);
  if (price) out.push(["price", price]);
  return out;
}

const VISION_QUERY = ["图", "读图", "看图", "vision", "vl"];

function matches(row: Folded, q: string): boolean {
  if (!q) return true;
  if (VISION_QUERY.some((v) => v === q)) return row.routes.some((r) => r.vision);
  if (row.model.toLowerCase().includes(q)) return true;
  return row.routes.some(
    (r) => r.provider.toLowerCase().includes(q) || (r.vendor ?? "").toLowerCase().includes(q),
  );
}

interface Props {
  models: ModelEntry[];
  current?: string;
  busy: string;
  onPick: (ref: string) => void;
}

export function Models({ models, current, busy, onPick }: Props) {
  const [q, setQ] = useState("");
  const groups = useMemo(() => groupModels(models), [models]);
  const query = q.trim().toLowerCase();

  const shown = groups
    .map((g) => ({ ...g, rows: g.rows.filter((r) => matches(r, query)) }))
    .filter((g) => g.rows.length > 0);
  const total = groups.reduce((n, g) => n + g.rows.length, 0);
  const hits = shown.reduce((n, g) => n + g.rows.length, 0);

  if (models.length === 0) return <div className="empty">读不到模型列表。</div>;

  return (
    <>
      {total > 8 && (
        <div className="mfind">
          <input
            type="search"
            value={q}
            spellCheck={false}
            placeholder="搜模型名、来源，或输入「图」只看能读图的…"
            aria-label="搜索模型"
            onChange={(e) => setQ(e.target.value)}
          />
          {query && (
            <span className="cnt">
              {hits} / {total}
            </span>
          )}
        </div>
      )}
      {shown.map((g) => (
        <div className="mgrp" key={g.key}>
          <div className="mgrp-hd">
            <span className="nm">{g.label}</span>
            {g.host && <span className="url">{g.host}</span>}
            <span className="n">{g.rows.length} 个模型</span>
          </div>
          {g.rows.map((row) => (
            <Row key={row.key} row={row} current={current} busy={busy} onPick={onPick} />
          ))}
        </div>
      ))}
      {shown.length === 0 && <div className="empty">没有匹配的模型。</div>}
    </>
  );
}

function routeLabel(m: ModelEntry): string {
  return KIND_LABEL[m.kind ?? ""] ?? m.kind ?? m.provider;
}

function Row({
  row, current, busy, onPick,
}: {
  row: Folded; current?: string; busy: string; onPick: (ref: string) => void;
}) {
  // The route carrying the current selection is the one to show; otherwise the
  // first, so an unselected row still names what clicking it would use.
  const active = row.routes.find((r) => r.ref === current);
  const shown = active ?? row.routes[0];
  const tags = tagsFor(shown);
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setOpen(false), []);
  useDismiss(open, box, close);

  return (
    <div className="mrow" data-on={active ? "" : undefined}>
      <button className="pick" disabled={busy !== ""} onClick={() => onPick(shown.ref)}>
        <span className="mark" />
        <span className="nm">{row.model}</span>
        <span className="caps">
          {tags.map(([k, t]) => (
            <i className="cap" data-k={k} key={k}>
              {t}
            </i>
          ))}
        </span>
      </button>
      {/* A native select loses its dropdown to any focus change, and this pane
          re-renders behind it. The role band's popover does not, so the two
          pickers in this pane are the same control. */}
      {row.routes.length > 1 && (
        <div className="viabox" ref={box}>
          <button
            className="via"
            aria-expanded={open}
            aria-haspopup="listbox"
            aria-label={`${row.model} 经由哪个协议`}
            disabled={busy !== ""}
            onClick={() => setOpen((v) => !v)}
          >
            经由 {routeLabel(shown)}
          </button>
          {open && (
            <div className="rpick" role="listbox" aria-label={`${row.model} 的协议路由`}>
              {row.routes.map((r) => (
                <button
                  key={r.ref}
                  role="option"
                  aria-selected={r.ref === shown.ref}
                  data-cur={r.ref === shown.ref ? "" : undefined}
                  onClick={() => {
                    setOpen(false);
                    onPick(r.ref);
                  }}
                >
                  {routeLabel(r)}
                  <span className="sub">{r.provider}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
