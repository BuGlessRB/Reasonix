import { useEffect, useState } from "react";
import { t } from "../i18n";
import type { AgentPort, MemoryEntry } from "../port/port";

// Memory is the only thing here that changes how the agent behaves without the
// user ever configuring it — the agent writes it. So the grouping answers the
// question that actually gets asked: when does this one apply?
const GROUPS: [string, string, string][] = [
  ["pinned", "一直生效", "每一轮都在提示词里，等同于你给它的长期指令"],
  ["relevant", "相关时才被想起", "只有这一轮看起来相关时才会被翻出来"],
];

const SCOPE: Record<string, string> = { project: "项目", global: "我的" };

export function Memory({ port }: { port: AgentPort }) {
  const [items, setItems] = useState<MemoryEntry[] | null>(null);
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");

  const reload = () => {
    port
      .memories()
      .then((c) => {
        setItems(c.memories);
        setQuery(c.recallQuery);
      })
      .catch(() => setItems(null));
  };
  useEffect(reload, [port]); // eslint-disable-line react-hooks/exhaustive-deps

  if (!items) return <div className="empty">{t("读不到记忆。")}</div>;
  if (items.length === 0) return <div className="empty">{t("还没有记下任何东西。")}</div>;

  const forget = async (name: string) => {
    setBusy(name);
    setError("");
    try {
      await port.forgetMemory(name);
      reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  };

  const usedCount = items.filter((m) => m.usedLastTurn).length;

  return (
    <div className="mem">
      {query && (
        <p className="recall">
          {t("上一轮从「{q}」出发翻了一次记忆", { q: clip(query) })}
          {usedCount > 0 ? t("，用上了 {n} 条", { n: usedCount }) : t("，一条都没用上")}
        </p>
      )}
      {GROUPS.map(([id, label, desc]) => {
        const group = items.filter((m) => m.activation === id);
        if (group.length === 0) return null;
        return (
          <section className="memgrp" key={id}>
            <div className="hd">
              <span className="lb">{t(label)}</span>
              <span className="c">{group.length}</span>
            </div>
            <p className="ds">{t(desc)}</p>
            {group.map((m) => (
              <div className="memrow" key={m.name} data-used={m.usedLastTurn ? "" : undefined}>
                <div className="line">
                  <i className="dot" title={m.usedLastTurn ? t("上一轮用上了") : undefined} />
                  <button className="nm" onClick={() => setOpen(open === m.name ? "" : m.name)}>
                    {m.title || m.name}
                  </button>
                  <span className="ds">{m.description}</span>
                  {m.expired && <i className="stale">{t("已过期")}</i>}
                  <span className="sc">{t(SCOPE[m.scope ?? ""] ?? m.scope ?? "")}</span>
                  <span className="at">{m.updatedAt || m.createdAt}</span>
                  <button className="act ghost" disabled={busy === m.name} onClick={() => void forget(m.name)}>
                    {t(busy === m.name ? "…" : "忘掉")}
                  </button>
                </div>
                {m.usedLastTurn && m.why && <div className="why-used">{t("上一轮因为「{why}」被翻出来", { why: m.why })}</div>}
                {open === m.name && (
                  <div className="peek">
                    <pre>{m.body?.trim() || "（没有正文）"}</pre>
                    {m.path && <span className="path">{m.path}</span>}
                  </div>
                )}
              </div>
            ))}
          </section>
        );
      })}
      {error && <div className="why">{error}</div>}
    </div>
  );
}

function clip(s: string): string {
  const t = s.trim();
  return t.length > 24 ? t.slice(0, 24) + "…" : t;
}
