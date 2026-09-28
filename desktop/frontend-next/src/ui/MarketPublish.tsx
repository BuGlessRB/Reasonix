import { useEffect, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AgentPort, MarketKind, MarketPackage, MarketPublished } from "../port/port";

const KINDS: [MarketKind, string][] = [["skill", "技能"], ["plugin", "插件"], ["mcp", "MCP 服务"], ["theme", "主题"]];

// What a source has to look like for the market to install it once approved;
// the kernel refuses anything else before it leaves the machine.
const PINNED_TREE = "固定到提交的 GitHub 目录，形如 https://github.com/owner/repo/tree/（40 位提交号）/子目录";
const SOURCE_TIP: Record<MarketKind, string> = {
  skill: "SKILL.md 的 https 地址，或 GitHub 仓库里某个技能的目录。",
  plugin: PINNED_TREE,
  mcp: ".mcp.json 或仓库的 https 地址，或 npm 包名。",
  theme: PINNED_TREE,
};

const STATUS: Record<string, [string, string | undefined]> = {
  pending: ["审核中", undefined],
  active: ["已公开", "ok"],
  rejected: ["未通过", "err"],
  hidden: ["已隐藏", undefined],
};

interface Draft {
  kind: MarketKind;
  name: string;
  source: string;
  summary: string;
  description: string;
  repoUrl: string;
  version: string;
  tags: string;
}

const EMPTY: Draft = { kind: "skill", name: "", source: "", summary: "", description: "", repoUrl: "", version: "", tags: "" };

// The form only collects; which sources are publishable and what the registry
// accepts are the kernel's and the registry's answers, shown as they come back.
export function PublishForm({ port, handle, onMine }: { port: AgentPort; handle: string; onMine: () => void }) {
  const [d, setD] = useState<Draft>(EMPTY);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState<MarketPublished | null>(null);
  const set = (k: keyof Draft) => (e: { target: { value: string } }) => setD({ ...d, [k]: e.target.value });

  const submit = async () => {
    setBusy(true);
    setError("");
    try {
      setDone(
        await port.publishMarket({
          kind: d.kind, name: d.name, source: d.source, summary: d.summary, description: d.description,
          repoUrl: d.repoUrl, version: d.version, tags: d.tags.split(/[,，]/).map((x) => x.trim()).filter(Boolean),
        }),
      );
    } catch (e) {
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  };

  if (done) {
    return (
      <div className="mkt mkt-pub" data-stage="done">
        <div className="find" data-lvl="ok">
          <span className="t">{t("已提交 {slug} {version}", { slug: done.package.slug, version: done.version })}</span>
          <span className="why">{t("审核通过后会在社区市场公开；在「我的发布」里可以看到审核进度。")}</span>
        </div>
        <div className="acts">
          <button className="act" data-action="market.publish-again" onClick={() => { setD(EMPTY); setDone(null); }}>
            {t("再发布一个")}
          </button>
          <button className="act" data-action="market.view" data-value="mine" data-primary onClick={onMine}>
            {t("查看我的发布")}
          </button>
        </div>
      </div>
    );
  }

  const ready = d.name.trim() !== "" && d.source.trim() !== "" && !busy;
  return (
    <div className="mkt mkt-pub">
      <p className="mkt-sum">{t("以 @{handle} 的名义提交，审核通过后公开。只收来源地址，不上传文件。", { handle })}</p>
      <div className="seg" data-text role="radiogroup" aria-label={t("类型")}>
        {KINDS.map(([id, name]) => (
          <button key={id} role="radio" aria-checked={d.kind === id} data-action="market.draft" data-value="kind" onClick={() => setD({ ...d, kind: id })}>
            {t(name)}
          </button>
        ))}
      </div>
      {d.kind === "theme" && <p className="mkt-tip">{t("主题以插件包发布，包里只能有主题；带技能、钩子或 MCP 服务的包请按插件发布。")}</p>}
      <div className="mkt-fields">
        <label>
          <span>{t("名称")}</span>
          <input value={d.name} data-action="market.draft" data-value="name" placeholder="my-package" spellCheck={false} onChange={set("name")} />
          <em className="mkt-tip">{t("小写字母、数字、点、下划线、连字符，最多 64 个字符。")}</em>
        </label>
        <label>
          <span>{t("版本")}</span>
          <input value={d.version} data-action="market.draft" data-value="version" placeholder="0.1.0" spellCheck={false} onChange={set("version")} />
          <em className="mkt-tip">{t("留空时新包为 0.1.0，更新自动加一个补丁号。")}</em>
        </label>
        <label className="full">
          <span>{t("来源地址")}</span>
          <input className="mono" value={d.source} data-action="market.draft" data-value="source" spellCheck={false} onChange={set("source")} />
          <em className="mkt-tip">{t(SOURCE_TIP[d.kind])}</em>
        </label>
        <label className="full">
          <span>{t("摘要")}</span>
          <input value={d.summary} data-action="market.draft" data-value="summary" maxLength={200} onChange={set("summary")} />
        </label>
        <label className="full">
          <span>{t("描述")}</span>
          <textarea rows={4} value={d.description} data-action="market.draft" data-value="description" maxLength={8000} onChange={set("description")} />
        </label>
        <label>
          <span>{t("仓库")}</span>
          <input className="mono" value={d.repoUrl} data-action="market.draft" data-value="repoUrl" placeholder="https://github.com/…" spellCheck={false} onChange={set("repoUrl")} />
        </label>
        <label>
          <span>{t("标签")}</span>
          <input value={d.tags} data-action="market.draft" data-value="tags" placeholder={t("用逗号分隔，最多 8 个")} onChange={set("tags")} />
        </label>
      </div>
      {error && (
        <div className="find" data-lvl="err">
          <span className="t">{t("没有提交成功")}</span>
          <span className="why">{error}</span>
        </div>
      )}
      <div className="acts">
        <span className="note">{t("提交后进入审核队列；审核员会固定审核时的内容，之后只安装那一份。")}</span>
        <button className="act" data-action="market.publish" data-primary disabled={!ready} onClick={() => void submit()}>
          {t(busy ? "提交中…" : "提交审核")}
        </button>
      </div>
    </div>
  );
}

export function MyPackages({ port }: { port: AgentPort }) {
  const [rows, setRows] = useState<MarketPackage[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    port.myMarket().then(setRows).catch((e) => {
      setError(reason(e));
      setRows([]);
    });
  }, [port]);

  return (
    <div className="mkt">
      {error && (
        <div className="find" data-lvl="err">
          <span className="t">{t("无法读取我的发布")}</span>
          <span className="why">{error}</span>
        </div>
      )}
      {rows === null && <div className="empty">{t("正在读取…")}</div>}
      {rows?.length === 0 && !error && <div className="empty">{t("还没有发布过。")}</div>}
      <ul className="mkt-list">
        {rows?.map((p) => {
          const [label, tone] = STATUS[p.status] ?? [p.status, undefined];
          return (
            <li key={p.slug} className="mkt-row" data-static="">
              <span className="mkt-hd">
                <span className="nm">{p.name}</span>
                <span className="mkt-kind">{t(KINDS.find(([k]) => k === p.kind)?.[1] ?? p.kind)}</span>
                <span className="mkt-badge" data-tone={tone}>{t(label)}</span>
              </span>
              {p.summary && <span className="mkt-sum">{p.summary}</span>}
              <span className="mkt-meta">
                <span className="mkt-id">{`@${p.handle} · v${p.latestVersion}`}</span>
                {p.status === "active" && <> · {t("{n} 次安装", { n: p.installCount })}</>}
              </span>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
