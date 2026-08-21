import { useMemo, useState } from "react";
import { t } from "../i18n";

// Which models a connection offers. A gateway answers with hundreds of names
// that differ by a date suffix, so the list is a search field first: one field
// that filters, and — when nothing on the list carries the name — adds it. Two
// boxes would be two controls for one intent, and the user would have to know
// which one their model is behind.

// 一行一个 DOM 节点的列表要有上限（perf/README 的判据）。上限只砍没勾的尾巴：
// 勾上的永远列出来，否则"我选了什么"会被截断，而那是这个面板要回答的问题。
const CAP = 60;

interface Props {
  // Everything on offer: what the endpoint reported, plus anything added here.
  models: string[];
  picked: string[];
  vision: string[];
  // Absent where the flow has no default to set — adding a source takes the
  // first pick, so there is nothing to choose yet.
  onDefault?: (m: string) => void;
  def?: string;
  // Absent where the answer is the endpoint's and not the user's to change.
  onVision?: (m: string) => void;
  // The endpoint refuses image input, so the switch would be a dead control.
  visionLocked?: boolean;
  onToggle: (m: string) => void;
  onAdd: (m: string) => void;
}

export function ModelChoice({
  models, picked, vision, def, onDefault, onVision, visionLocked, onToggle, onAdd,
}: Props) {
  const [q, setQ] = useState("");
  const query = q.trim().toLowerCase();
  const on = useMemo(() => new Set(picked), [picked]);

  // Enabled first, and deliberately keyed on the list alone: re-sorting on every
  // tick throws the row out from under the pointer that just hit it. Menu.tsx
  // keeps group order over relevance for the same reason.
  const order = useMemo(
    () => [...models].sort((a, b) => Number(picked.includes(b)) - Number(picked.includes(a))),
    [models],
  );

  const hits = query ? order.filter((m) => m.toLowerCase().includes(query)) : order;
  const [shown, hidden] = cap(hits, on);
  // Only when nothing already carries the name: offering to add a model that is
  // sitting three rows down is how a list grows two of everything.
  const naming = q.trim() !== "" && !models.some((m) => m.toLowerCase() === query);

  const add = () => {
    onAdd(q.trim());
    setQ("");
  };

  return (
    <>
      <div className="mfind">
        <input
          type="search"
          value={q}
          spellCheck={false}
          placeholder={t("搜模型名；列表里没有的，直接把名字写在这里")}
          aria-label={t("搜索或添加模型")}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== "Enter" || !naming) return;
            e.preventDefault();
            add();
          }}
        />
        {query && <span className="cnt">{hits.length} / {models.length}</span>}
      </div>

      <div className="mrows">
        {shown.map((m) => (
          <div className="mline" key={m} data-off={on.has(m) ? undefined : ""}>
            <button className="tick" role="checkbox" aria-checked={on.has(m)}
              aria-label={t("选用 {name}", { name: m })} onClick={() => onToggle(m)}>
              <i />
            </button>
            <span className="nm">{m}</span>
            {onVision ? (
              <button className="vtag" aria-pressed={vision.includes(m)}
                disabled={!on.has(m) || visionLocked}
                title={visionLocked ? t("内核不给这个端点发图片，改这里不会有效果") : undefined}
                onClick={() => onVision(m)}>
                {t("读图")}
              </button>
            ) : (
              vision.includes(m) && <span className="vtag" data-flat>{t("读图")}</span>
            )}
            {onDefault && (
              <button className="dtag" aria-pressed={def === m} disabled={!on.has(m)}
                onClick={() => onDefault(m)}>
                {t("默认")}
              </button>
            )}
          </div>
        ))}
      </div>

      {hidden > 0 && <span className="mmore">{t("还有 {n} 个没列出来，搜一下就能找到", { n: hidden })}</span>}
      {shown.length === 0 && !naming && <div className="empty">{t("没有匹配的模型。")}</div>}

      {/* Under the list, never over it: a "create" offer sitting above seven
          real matches is how you add a model when you meant to pick one. */}
      {naming && (
        <button className="mnew" onClick={add}>
          <span className="k">+</span>
          <span className="tx">
            <span className="lb">{t("就用「{name}」", { name: q.trim() })}</span>
            <span className="ds">{t("端点没报出这个名字，能不能用只有你知道")}</span>
          </span>
        </button>
      )}
    </>
  );
}

function cap(hits: string[], on: Set<string>): [string[], number] {
  if (hits.length <= CAP) return [hits, 0];
  let room = Math.max(CAP - hits.filter((m) => on.has(m)).length, 0);
  const out = hits.filter((m) => on.has(m) || room-- > 0);
  return [out, hits.length - out.length];
}
