import { useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import { reason } from "../i18n/kernel";
import type { AccountState, AgentPort, MarketDetail, MarketKind, MarketPackage, MarketPlan } from "../port/port";
import { Outcome } from "./AddPlugin";
import { Group } from "./Group";
import { PlanConfirm } from "./MarketConfirm";
import { MyPackages, PublishForm } from "./MarketPublish";
import { arrowTabs } from "./tablist";

const KINDS: [MarketKind | "", string][] = [["", "全部"], ["skill", "技能"], ["plugin", "插件"], ["mcp", "MCP 服务"], ["theme", "主题"]];
type Sort = "trending" | "installs" | "new";
const SORTS: [Sort, string][] = [["trending", "近期热门"], ["installs", "安装最多"], ["new", "最新"]];
const KIND_NAME: Record<string, string> = { skill: "技能", plugin: "插件", mcp: "MCP 服务", theme: "主题" };

interface Props {
  port: AgentPort;
  onInstalled: () => void;
}

// The market is one more place a source comes from. It lists what reviewers let
// through and hands the approved version to the same plan-then-install every
// pasted address goes through; the kernel holds the pin, this only shows it.
export function Market({ port, onInstalled }: Props) {
  const [kind, setKind] = useState<MarketKind | "">("");
  const [sort, setSort] = useState<Sort>("trending");
  const [q, setQ] = useState("");
  const [rows, setRows] = useState<MarketPackage[] | null>(null);
  const [more, setMore] = useState(false);
  const [error, setError] = useState("");
  const [open, setOpen] = useState("");
  const asked = useRef(0);

  const load = (offset: number) => {
    const n = ++asked.current;
    setError("");
    port
      .marketList({ kind, q: q.trim(), sort, offset })
      .then((page) => {
        if (n !== asked.current) return;
        setRows((prev) => (offset > 0 && prev ? [...prev, ...page.packages] : page.packages));
        setMore(page.packages.length >= page.limit);
      })
      .catch((e) => {
        if (n !== asked.current) return;
        setError(reason(e));
        if (offset === 0) setRows([]);
      });
  };

  useEffect(() => {
    const timer = setTimeout(() => load(0), q ? 250 : 0);
    return () => clearTimeout(timer);
  }, [kind, sort, q]); // eslint-disable-line react-hooks/exhaustive-deps

  if (open) {
    return (
      <Entry
        port={port}
        slug={open}
        onBack={() => setOpen("")}
        onInstalled={() => {
          onInstalled();
          load(0);
        }}
      />
    );
  }

  return (
    <div className="mkt">
      <div className="mkt-bar">
        <input
          className="mkt-q"
          type="search"
          data-action="market.search"
          value={q}
          placeholder={t("搜索技能、插件、MCP 服务与主题")}
          aria-label={t("搜索社区市场")}
          onChange={(e) => setQ(e.target.value)}
        />
        <div className="seg" data-text role="radiogroup" aria-label={t("类型")}>
          {KINDS.map(([id, name]) => (
            <button key={id || "all"} role="radio" aria-checked={kind === id} data-action="market.kind" data-value={id || "all"} onClick={() => setKind(id)}>
              {t(name)}
            </button>
          ))}
        </div>
        <div className="seg" data-text role="radiogroup" aria-label={t("排序")}>
          {SORTS.map(([id, name]) => (
            <button key={id} role="radio" aria-checked={sort === id} data-action="market.sort" data-value={id} onClick={() => setSort(id)}>
              {t(name)}
            </button>
          ))}
        </div>
      </div>
      {error && (
        <div className="find" data-lvl="err">
          <span className="t">{t("无法读取社区市场")}</span>
          <span className="why">{error}</span>
        </div>
      )}
      {rows === null && !error && <div className="empty">{t("正在读取…")}</div>}
      {rows?.length === 0 && !error && <div className="empty">{t("没有找到匹配的包。")}</div>}
      <ul className="mkt-list">
        {rows?.map((p) => (
          <li key={p.slug}>
            <button className="mkt-row" data-action="market.open" data-value={p.slug} onClick={() => setOpen(p.slug)}>
              <span className="mkt-hd">
                <span className="nm">{p.name}</span>
                <span className="mkt-kind">{t(KIND_NAME[p.kind] ?? p.kind)}</span>
                {p.verified && <span className="mkt-badge" data-tone="ok">{t("已验证")}</span>}
                {p.installed && (
                  <span className="mkt-badge">
                    {p.installed.version === p.latestVersion ? t("已安装") : t("可更新")}
                  </span>
                )}
              </span>
              {p.summary && <span className="mkt-sum">{p.summary}</span>}
              <span className="mkt-meta">
                <span className="mkt-id">{`@${p.handle} · v${p.latestVersion}`}</span> · {t("{n} 次安装", { n: p.installCount })}
              </span>
            </button>
          </li>
        ))}
      </ul>
      {more && (
        <button className="act" data-action="market.more" onClick={() => load(rows?.length ?? 0)}>
          {t("加载更多")}
        </button>
      )}
    </div>
  );
}

function Entry({ port, slug, onBack, onInstalled }: { port: AgentPort; slug: string; onBack: () => void; onInstalled: () => void }) {
  const [d, setD] = useState<MarketDetail | null>(null);
  const [plan, setPlan] = useState<MarketPlan | null>(null);
  const [done, setDone] = useState<MarketPlan | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    port.marketDetail(slug).then(setD).catch((e) => setError(reason(e)));
  }, [port, slug]);

  const update = !!d?.installed && d.installed.version !== d.package.latestVersion;
  const look = async () => {
    setBusy(true);
    setError("");
    try {
      setPlan(await port.planMarket({ slug, replace: update }));
    } catch (e) {
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  };
  const install = async () => {
    if (!plan) return;
    setBusy(true);
    setError("");
    try {
      const out = await port.installMarket({ slug, version: plan.version, planId: plan.planId, replace: update });
      setDone(out);
      if (out.applied) onInstalled();
    } catch (e) {
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  };

  const back = (
    <button className="act" data-action="market.back" onClick={onBack}>
      {t("返回列表")}
    </button>
  );

  if (done) {
    return (
      <div className="mkt addpkg" data-stage="done">
        <Outcome plan={done} />
        <div className="acts">{back}</div>
      </div>
    );
  }

  if (plan) {
    return <PlanConfirm key={plan.planId} slug={slug} plan={plan} busy={busy} error={error} onCancel={() => setPlan(null)} onInstall={() => void install()} />;
  }

  if (!d) {
    return (
      <div className="mkt">
        {error ? (
          <div className="find" data-lvl="err">
            <span className="t">{t("无法读取 {name}", { name: slug })}</span>
            <span className="why">{error}</span>
          </div>
        ) : (
          <div className="empty">{t("正在读取…")}</div>
        )}
        <div className="acts">{back}</div>
      </div>
    );
  }

  const p = d.package;
  const v = d.approved;
  const current = !!d.installed && !update;
  // A copied skill is never overwritten in place, so an update has to start
  // from its removal rather than fail at the last step.
  const stuck = update && p.kind === "skill";
  return (
    <div className="mkt mkt-entry">
      <div className="mkt-hd">
        <span className="nm">{p.name}</span>
        <span className="mkt-kind">{t(KIND_NAME[p.kind] ?? p.kind)}</span>
        {p.verified && <span className="mkt-badge" data-tone="ok">{t("已验证")}</span>}
      </div>
      {p.summary && <p className="mkt-sum">{p.summary}</p>}
      {p.description && <p className="mkt-desc">{p.description}</p>}
      <dl className="mkt-facts">
        <dt>{t("发布者")}</dt>
        <dd>@{p.handle}</dd>
        <dt>{t("审核版本")}</dt>
        <dd>{v?.version || p.latestVersion}</dd>
        <dt>{t("来源")}</dt>
        <dd className="mono">{v?.source || "—"}</dd>
        <dt>{t("固定内容")}</dt>
        <dd className={d.pinned ? "mono" : undefined} data-missing={d.pinned ? undefined : ""}>{d.pinned ? v?.contentHash : t("未固定——审核时没有记录内容摘要")}</dd>
        {p.repoUrl && (
          <>
            <dt>{t("仓库")}</dt>
            <dd className="mono">{p.repoUrl}</dd>
          </>
        )}
        {p.tags.length > 0 && (
          <>
            <dt>{t("标签")}</dt>
            <dd>{p.tags.join(" · ")}</dd>
          </>
        )}
      </dl>
      <div className="acts">
        <span className="note">
          {current
            ? t("已安装 {version}", { version: d.installed!.version })
            : stuck
              ? t("技能不会被原地覆盖：先在「已安装」里移除旧版本，再回来安装")
              : d.pinned
              ? t("先列出将安装的全部内容，确认后才会写入")
              : t("审核版本没有固定内容，不能从市场安装")}
        </span>
        {back}
        {!current && !stuck && d.pinned && (
          <button className="act" data-action="market.inspect" data-primary disabled={busy} onClick={() => void look()}>
            {t(busy ? "读取中…" : update ? "查看更新内容" : "查看将安装的内容")}
          </button>
        )}
      </div>
      {error && <div className="why">{error}</div>}
    </div>
  );
}

type View = "browse" | "mine" | "publish";
const VIEWS: [View, string][] = [["browse", "浏览"], ["mine", "我的发布"], ["publish", "发布"]];

// Installed and discover are two views of one subject, so they are tabs of one
// page rather than two sections: what the market adds shows up on the other tab.
// Publishing spends the account session, so it is offered only while signed in.
export function MarketGroup({ port, onInstalled, account, onSignIn }: Props & { account: AccountState | null; onSignIn: () => void }) {
  const [view, setView] = useState<View>("browse");
  const handle = account?.signedIn ? account.user?.handle : undefined;
  const at = handle ? view : "browse";
  return (
    <Group id="market" title={t("社区市场")}
      hint={t("社区发布、经过审核的技能、插件、MCP 服务与主题。只有固定了审核内容的版本才能安装；安装前会列出将写入的全部内容，与粘贴地址安装走同一套确认。")}>
      {handle ? (
        <div className="seg mkt-views" data-text role="radiogroup" aria-label={t("社区市场")}>
          {VIEWS.map(([id, name]) => (
            <button key={id} role="radio" aria-checked={at === id} data-action="market.view" data-value={id} onClick={() => setView(id)}>
              {t(name)}
            </button>
          ))}
        </div>
      ) : (
        account !== null && (
          <div className="mkt-signin">
            <span>{t("登录后可以在这里发布技能、插件、MCP 服务和主题，并查看审核进度。")}</span>
            <button className="act" data-action="market.signin" onClick={onSignIn}>
              {t("去登录")}
            </button>
          </div>
        )
      )}
      {at === "browse" && <Market port={port} onInstalled={onInstalled} />}
      {at === "mine" && <MyPackages port={port} onInstalled={onInstalled} />}
      {at === "publish" && handle && <PublishForm port={port} handle={handle} onMine={() => setView("mine")} />}
    </Group>
  );
}

export function ExtTabs({ at, onPick }: { at: "installed" | "market"; onPick: (at: "installed" | "market") => void }) {
  const tabs: ["installed" | "market", string][] = [["installed", "已安装"], ["market", "发现"]];
  return (
    <div className="seg mkt-tabs" data-text role="tablist" aria-label={t("扩展")} onKeyDown={arrowTabs}>
      {tabs.map(([id, name]) => (
        <button key={id} role="tab" aria-selected={at === id} tabIndex={at === id ? 0 : -1} data-action="extensions.tab" data-value={id} onClick={() => onPick(id)}>
          {t(name)}
        </button>
      ))}
    </div>
  );
}
