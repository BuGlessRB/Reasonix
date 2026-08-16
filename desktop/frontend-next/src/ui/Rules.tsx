import { useEffect, useState } from "react";
import { t } from "../i18n";
import type { AgentPort, PermissionLists, PermissionRules } from "../port/port";
import { Switch } from "./Switch";

type List = "deny" | "ask" | "allow";

// The three lists in the order the gate reads them, each described by what it
// does to a call that matches — not by what the field is called.
const LISTS: [List, string, string][] = [
  ["deny", "一律拒绝", "先看这一组。命中了就没有商量的余地：全放行也拦得住，批准框不会出现。"],
  ["ask", "每次都问", "命中的调用一定停下来等你，哪怕自动批准开着。"],
  ["allow", "直接放行", "命中的调用不再问你。没命中的按下面那档处理。"],
];

const MODES: [string, string, string][] = [
  ["ask", "问我", "没被上面三组说中的写操作，动手前问一次"],
  ["allow", "放行", "没被说中的写操作直接做"],
  ["deny", "拒绝", "没被说中的写操作一律不做"],
];

// The rules people actually want, written the way the gate parses them.
// `file_mutation` is every tool that writes a file; a bash rule matches by
// command fields, so `git push:*` cannot be slipped past with an argument.
interface Recipe {
  id: string;
  title: string;
  desc: string;
  list: List;
  rules: string[];
}

const RECIPES: Recipe[] = [
  {
    id: "secrets",
    title: "不许动 .env",
    desc: "文件工具读或写 .env 一律拒绝。bash 里的 cat 走的是另一条路，这条管不到它",
    list: "deny",
    rules: ["file_mutation(*.env*)", "read_file(*.env*)"],
  },
  {
    id: "push",
    title: "不许推到远端",
    desc: "本地怎么改都行，推出去这一步永远留给你",
    list: "deny",
    rules: ["bash(git push:*)"],
  },
  {
    id: "history",
    title: "不许改写 git 历史",
    desc: "rebase 和 reset 能吃掉你还没推的提交，这两个命令直接拒绝",
    list: "deny",
    rules: ["bash(git rebase:*)", "bash(git reset:*)"],
  },
  {
    id: "tests",
    title: "跑测试不用问",
    desc: "测试命令直接放行；其余命令照旧",
    list: "allow",
    rules: ["bash(go test:*)", "bash(npm test:*)", "bash(pytest:*)"],
  },
];

export function Rules({ port, onChanged }: { port: AgentPort; onChanged: () => void }) {
  const [rules, setRules] = useState<PermissionRules | null>(null);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [expert, setExpert] = useState(false);

  useEffect(() => {
    port.permissions().then(setRules).catch(() => setRules(null));
  }, [port]);

  if (!rules) return <div className="empty">{t("读不到权限配置。")}</div>;

  const lists: PermissionLists = { mode: rules.mode, deny: rules.deny, ask: rules.ask, allow: rules.allow };
  const count = rules.deny.length + rules.ask.length + rules.allow.length;

  // Every edit saves and rebuilds, so there is never an unsaved screen. The
  // kernel refuses the rebuild while a turn runs, and that refusal is the
  // message — a rule that only exists on screen is worse than no rule.
  const apply = async (what: string, next: PermissionLists) => {
    setBusy(what);
    setError("");
    try {
      setRules(await port.savePermissions(next));
      onChanged();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  };

  const has = (r: Recipe) => r.rules.every((rule) => lists[r.list].includes(rule));
  const toggle = (r: Recipe) => {
    const on = has(r);
    const next = on
      ? lists[r.list].filter((rule) => !r.rules.includes(rule))
      : [...lists[r.list], ...r.rules.filter((rule) => !lists[r.list].includes(rule))];
    void apply(r.id, { ...lists, [r.list]: next });
  };

  return (
    <div className="rules">
      {rules.shadowedBy && (
        <div className="find" data-lvl="warn" role="status">
          <span className="t">{t("这个项目自带一份权限配置")}</span>
          <span className="why">
            {t("{path} 里也写了 permissions，实际生效的是它。这里的改动会存下来，但要等它不再声明才用得上。", {
              path: rules.shadowedBy,
            })}
          </span>
        </div>
      )}

      <div className="recipes">
        {RECIPES.map((r) => (
          <div className="recipe" key={r.id} data-on={has(r) ? "" : undefined}>
            <div className="tx">
              <span className="lb">{t(r.title)}</span>
              <span className="ds">{t(r.desc)}</span>
            </div>
            <Switch on={has(r)} busy={busy === r.id} label={t(r.title)} onClick={() => toggle(r)} />
          </div>
        ))}
      </div>

      <div className="fallback">
        <span className="k">{t("剩下的写操作")}</span>
        <div className="seg" data-text role="radiogroup" aria-label={t("剩下的写操作")}>
          {MODES.map(([id, label]) => (
            <button
              key={id}
              role="radio"
              aria-checked={rules.mode === id}
              disabled={!!busy}
              onClick={() => void apply("mode", { ...lists, mode: id })}
            >
              {t(label)}
            </button>
          ))}
        </div>
      </div>
      <p className="note">{t(MODES.find(([id]) => id === rules.mode)?.[2] ?? "")}</p>

      <button className="more" aria-expanded={expert} onClick={() => setExpert((v) => !v)}>
        {t(expert ? "收起" : "自己写一条")}
        <span className="c">{t("{n} 条规则", { n: count })}</span>
      </button>

      {expert && (
        <div className="expert">
          {LISTS.map(([id, label, why]) => (
            <RuleList
              key={id}
              label={t(label)}
              why={t(why)}
              kind={id}
              rules={lists[id]}
              inherited={rules.effective ? rules.effective[id].filter((r) => !lists[id].includes(r)) : []}
              busy={!!busy}
              onChange={(next) => void apply(id, { ...lists, [id]: next })}
            />
          ))}
          <p className="note">
            {t("写法：工具名，或者工具名(要匹配的东西)。bash 按命令的词来比，所以 bash(git push:*) 挡得住带任何参数的 git push；文件工具比的是路径，* 能跨过 /。file_mutation 代表所有会写文件的工具。")}
          </p>
          <p className="path">{rules.path}</p>
        </div>
      )}
      {error && <div className="why">{error}</div>}
    </div>
  );
}

function RuleList({
  label, why, kind, rules, inherited, busy, onChange,
}: {
  label: string;
  why: string;
  kind: List;
  rules: string[];
  inherited: string[];
  busy: boolean;
  onChange: (next: string[]) => void;
}) {
  const [draft, setDraft] = useState("");
  const add = () => {
    const rule = draft.trim();
    if (!rule || rules.includes(rule)) return;
    setDraft("");
    onChange([...rules, rule]);
  };

  return (
    <section className="rlist" data-k={kind}>
      <div className="rhd">
        <span className="lb">{label}</span>
        <span className="ds">{why}</span>
      </div>
      {rules.map((rule) => (
        <div className="prule" key={rule}>
          <code>{rule}</code>
          <button className="act ghost" disabled={busy} aria-label={t("删掉 {rule}", { rule })}
            onClick={() => onChange(rules.filter((r) => r !== rule))}>
            {t("删掉")}
          </button>
        </div>
      ))}
      {/* A rule the project file brought is real and matches first, so hiding it
          would describe a gate the user does not have. It is not deletable from
          here, and the row says which file to go edit instead. */}
      {inherited.map((rule) => (
        <div className="prule" key={rule} data-ro="">
          <code>{rule}</code>
          <span className="sc">{t("来自项目配置")}</span>
        </div>
      ))}
      <div className="radd">
        <input
          value={draft}
          placeholder={t(kind === "allow" ? "例如 bash(go build:*)" : "例如 bash(rm:*)")}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && add()}
        />
        <button className="act" disabled={busy || !draft.trim()} onClick={add}>
          {t("加上")}
        </button>
      </div>
    </section>
  );
}
