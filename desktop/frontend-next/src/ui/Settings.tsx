import { useCallback, useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import { useRuntimeReload } from "./RuntimeReload";
import type { AccountState, AgentPort, Appearance as Look, ApprovalMode, CapabilityScope, McpEntry, ModelEntry, PluginPackage, Preset, RoleAssignments, SessionStatus, SkillEntry } from "../port/port";
import { arrowTabs } from "./tablist";
import { WindowControls } from "./WindowControls";
import { AddServer } from "./AddServer";
import { AddPlugin } from "./AddPlugin";
import { Packages } from "./Packages";
import { Switch } from "./Switch";
import { ServerRow } from "./ServerRow";
import { SkillRow } from "./SkillRow";
import { Hooks } from "./Hooks";
import { Network } from "./Network";
import { Shell as ShellPicker } from "./Shell";
import { Rules } from "./Rules";
import { Sandbox } from "./Sandbox";
import { Account } from "./Account";
import { Providers } from "./Providers";
import { Models, activeKind, groupVendors } from "./Models";
import { KIND_LABEL } from "./vendors";
import { planProtocolSwitch } from "./protocolswitch";
import { Roles } from "./Roles";
import { Boundary } from "./Boundary";
import { Versions } from "./Versions";
import { Memory } from "./Memory";
import { Storage } from "./Storage";
import { Appearance, SCHEMES } from "./Appearance";
import { ScopeBar } from "./CapabilityScope";

const PRESETS: [Preset, string, string][] = [
  ["balanced", "均衡", "做到模型认为做完为止。日常用这档"],
  ["delivery", "交付", "改了东西就得验证、复核、签收，少一样都不算做完"],
];

const APPROVALS: [ApprovalMode, string, string][] = [
  ["dontAsk", "不打扰", "不弹审批；要批准才能做的一概不做"],
  ["ask", "询问", "每次动手前问你"],
  ["auto", "自动", "低风险自己过，写操作仍然问"],
  ["yolo", "全放行", "不问了。只在你完全信任这个工作区时用"],
];

type Section = "session" | "model" | "tools" | "hooks" | "ext" | "network" | "memory" | "storage" | "account" | "versions" | "appearance" | "advanced";

// What still lives in the old desktop app. Bots are not on the roadmap, so
// they are not a promise to keep here either. Signing in and reading
// versions landed here; downloading and applying an update did not, and naming
// only that half keeps this list a fact rather than a promise.
// Everything that used to live here has a home now, so the 「高级」 tab filters
// itself out below. Keep the list rather than the tab: the next thing that is
// real but not built yet belongs in one line here, not in a half-made panel.
const ELSEWHERE: string[] = [];

const NAV: [Section, string][] = [
  ["session", "会话"],
  ["model", "模型"],
  ["tools", "工具与权限"],
  ["hooks", "自动化"],
  ["ext", "扩展"],
  ["network", "网络"],
  ["memory", "记忆"],
  ["storage", "存储"],
  ["account", "账号"],
  ["versions", "版本"],
  ["appearance", "外观"],
  ["advanced", "高级"],
].filter(([id]) => id !== "advanced" || ELSEWHERE.length > 0) as [Section, string][];

// The user's question is "is it there and does it work", so the state is the
// label. A failed server keeps its error on the row that names it.
const NET_MODE: Record<string, string> = { auto: "跟随系统", env: "环境变量", custom: "手动", off: "直连" };

interface Props {
  port: AgentPort;
  status: SessionStatus | null;
  theme: string;
  reloadThemes: () => void;
  onTheme: (t: string) => void;
  contrast: string;
  weight: string;
  onWeight: (v: string) => void;
  look: Look;
  onLook: (look: Look) => void;
  onContrast: (c: string) => void;
  onClose: () => void;
  onChanged: () => void;
  at?: string;
  account: AccountState | null;
  reloadAccount: () => void;
}

export function Settings({ port, status, theme, onTheme, contrast, onContrast, weight, onWeight, look, onLook, onClose, onChanged, reloadThemes, at: opened, account: acct, reloadAccount }: Props) {
  const [at, setAt] = useState<Section>((opened as Section) || "session");
  const [models, setModels] = useState<ModelEntry[]>([]);
  const [roles, setRoles] = useState<RoleAssignments | null>(null);
  const [protocol, setProtocol] = useState<Record<string, string>>({});
  const [mcp, setMcp] = useState<McpEntry[]>([]);
  const [scope, setScope] = useState<CapabilityScope | null>(null);
  const [scopes, setScopes] = useState<CapabilityScope[]>([]);
  // Which project the extension tab is answering for. Empty is the running one,
  // so the common case stays exactly what it was.
  const [scopeAt, setScopeAt] = useState("");
  const [skills, setSkills] = useState<SkillEntry[]>([]);
  const [implicit, setImplicit] = useState(true);
  const [live, setLive] = useState(true);
  const [busy, setBusy] = useState("");
  const [failed, setFailed] = useState("");
  const [adding, setAdding] = useState(false);
  const [packages, setPackages] = useState<PluginPackage[]>([]);
  const [addingPkg, setAddingPkg] = useState(false);
  const [updatingPkg, setUpdatingPkg] = useState("");
  const [hookCount, setHookCount] = useState(0);
  const [netMode, setNetMode] = useState("");
  const [memCount, setMemCount] = useState(0);
  const [ruleCount, setRuleCount] = useState(0);
  const root = useRef<HTMLDivElement>(null);

  const reloadExt = useCallback(() => {
    const where = scopeAt || undefined;
    port.capabilityScopes().then(setScopes).catch(() => setScopes([]));
    port
      .mcp(where)
      .then((c) => {
        setMcp(c.servers);
        setScope(c.scope);
        setLive(c.live !== false);
      })
      .catch(() => setMcp([]));
    port.plugins().then(setPackages).catch(() => setPackages([]));
    port.hooks().then((c) => setHookCount(c.hooks.length)).catch(() => setHookCount(0));
    port.network().then((n) => setNetMode(t(NET_MODE[n.mode] ?? n.mode))).catch(() => setNetMode(""));
    port.memories().then((c) => setMemCount(c.memories.length)).catch(() => setMemCount(0));
    port
      .permissions()
      .then((p) => setRuleCount(p.deny.length + p.ask.length + p.allow.length))
      .catch(() => setRuleCount(0));
    port
      .skills(where)
      .then((c) => {
        setSkills(c.skills);
        setImplicit(c.implicit);
      })
      .catch(() => setSkills([]));
  }, [port, scopeAt]);

  // An extension switch moves the metrics rail too, so the change has to leave
  // this pane as well as refresh it.
  const afterExtChange = useCallback(() => {
    reloadExt();
    onChanged();
  }, [reloadExt, onChanged]);

  const reload = useRuntimeReload(port, afterExtChange);

  // Adding or removing a source changes what the picker above can offer, so
  // the list is reloadable rather than read once at mount.
  const loadModels = useCallback(() => {
    port.models().then(setModels).catch(() => setModels([]));
  }, [port]);

  const loadRoles = useCallback(() => {
    port.roles().then(setRoles).catch(() => setRoles(null));
  }, [port]);

  // Focus lands once, when the pane opens. onClose is a fresh arrow on every
  // parent render, so re-running this with it would pull focus back out of
  // whatever the user is holding — a native select closes its dropdown the
  // instant it is blurred, which reads as the menu refusing to open at all.
  useEffect(() => {
    root.current?.focus();
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [onClose]);

  useEffect(() => {
    loadModels();
    loadRoles();
    reloadExt();
  }, [loadModels, loadRoles, reloadExt]);

  // A refused switch has to say so. The kernel turns one down while a turn or a
  // background job is running, and swallowing that leaves the click looking like
  // the row simply does not work.
  const run = async (what: string, fn: () => Promise<void>) => {
    setBusy(what);
    setFailed("");
    try {
      await fn();
      onChanged();
    } catch (e) {
      setFailed(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  };

  // The levels the selected model's endpoint actually accepts. A fixed list
  // here offered every model six of them and let the user set depths the
  // provider then ignored or rejected.
  const vendors = groupVendors(models);
  const kindFor = (key: string) => {
    const v = vendors.find((x) => x.key === key);
    return v ? activeKind(v, status?.modelRef) : "";
  };
  const switchProtocol = (key: string, kind: string) => {
    const plan = planProtocolSwitch(vendors.find((x) => x.key === key), models, status?.modelRef, kind);
    if (plan.do === "stay") {
      setFailed(t("{door} 这扇门上没有 {model}，先在下面挑一个它有的模型。", {
        door: t(KIND_LABEL[kind] ?? kind),
        model: plan.model,
      }));
      return;
    }
    setProtocol((p) => ({ ...p, [key]: kind }));
    if (plan.do === "switch") run(plan.ref, () => port.setModel(plan.ref));
  };
  const efforts = models.find((m) => m.ref === status?.modelRef)?.efforts ?? [];
  const assigned = roles ? Object.values(roles).filter(Boolean).length : 0;
  const preset = t(PRESETS.find(([id]) => id === status?.preset)?.[1] ?? "") || "—";
  const approval = t(APPROVALS.find(([id]) => id === status?.toolApprovalMode)?.[1] ?? "—");
  const broken = mcp.filter((m) => m.state === "failed").length;
  // A package owns what it brought, and its own row already lists it. What is
  // left is what the user added by hand, which is the only thing these two
  // lists can act on without contradicting the package above them. Ownership
  // is read off the packages themselves: a live server reports the config
  // layer it was merged from, which is not the same question.
  const owned = new Set(packages.flatMap((p) => p.mcpServers?.map((s) => s.name) ?? []));
  const looseMcp = mcp.filter((m) => !owned.has(m.name));
  const looseSkills = skills.filter((s) => !s.plugin);
  const looseOn = looseSkills.filter((s) => s.enabled).length;
  // The table of contents is also the status board: the value that matters for
  // each section rides on its own row, so the risky one is legible from here.
  const nav: Record<Section, string> = {
    session: status?.plan ? t("计划模式") : preset,
    model: status?.modelRef?.split("/").pop() ?? "—",
    tools: ruleCount ? `${approval} · ${ruleCount}` : approval,
    hooks: hookCount ? t("{n} 条", { n: hookCount }) : t("关"),
    ext: broken ? t("{n} 个异常", { n: broken }) : packages.length ? t("{n} 个包", { n: packages.length }) : `${looseMcp.length + looseOn}`,
    network: netMode,
    memory: memCount ? t("{n} 条", { n: memCount }) : "",
    storage: "",
    account: acct === null ? "" : acct.signedIn ? (acct.user?.label ?? t("已登录")) : t("未登录"),
    versions: "",
    appearance: t(SCHEMES.find(([id]) => id === theme)?.[1] ?? ""),
    advanced: ELSEWHERE.length ? `${ELSEWHERE.length}` : "",
  };
  const danger = (id: Section) =>
    (id === "tools" && status?.toolApprovalMode === "yolo") || (id === "ext" && broken > 0);

  return (
    <div className="prefs" ref={root} tabIndex={-1}>
      <div className="prefs-hd">
        <button className="back" onClick={onClose}>
          ‹ {t("返回工作台")}
        </button>
        <span className="ttl">{t("设置")}</span>
        <span className="esc">esc</span>
        <WindowControls />
      </div>

      <div className="prefs-body">
        <nav className="prefs-nav" role="tablist" aria-label={t("设置分类")} onKeyDown={arrowTabs}>
          {NAV.map(([id, name]) => (
            <button key={id} id={`prefs-${id}`} role="tab" aria-selected={at === id} onClick={() => setAt(id)}>
              {t(name)}
              <span className="nv" title={nav[id] || undefined} data-danger={danger(id) ? "" : undefined}>
                {nav[id]}
              </span>
            </button>
          ))}
        </nav>

        <div className="prefs-main" role="tabpanel" aria-labelledby={`prefs-${at}`} data-sec={at}>
          <Boundary
            fallback={
              <div className="find" data-lvl="err" role="alert">
                <span className="t">{t("这个设置分区出错了")}</span>
                <span className="why">{t("其它分区和你的会话不受影响；关掉设置再打开可重试。")}</span>
              </div>
            }
          >
          <div className="prefs-col">
          {at === "session" && (
            <>
              <Group title={t("执行设定")} now={preset} hint={t("管的是「做完了」谁说了算。切档立刻生效，不重建运行时。")}>
                <div className="seg" data-text role="radiogroup" aria-label={t("执行设定")}>
                  {PRESETS.map(([id, name]) => (
                    <button key={id} role="radio" aria-checked={status?.preset === id} disabled={!!busy}
                      onClick={() => run(id, () => port.setPreset(id))}>
                      {t(name)}
                    </button>
                  ))}
                </div>
                <p className="note">{t(PRESETS.find(([id]) => id === status?.preset)?.[2] ?? "")}</p>
              </Group>
              {/* An on/off that reverses by clicking again is a switch, not two
                  options — the same control the recipes and the sandbox's
                  network egress use. */}
              <Group title={t("计划模式")} hint={t("开着的时候拿不到写权限：这不是提示词里的约定，是没给这个能力。")}>
                <div className="lrow">
                  <span className="tx">
                    <span className="lb">{t("只读地出计划")}</span>
                    <span className="ds">
                      {t(status?.plan ? "只读加出计划，你批准后核心自己关掉它" : "正常执行：批准过的写操作直接做")}
                    </span>
                  </span>
                  <Switch
                    on={status?.plan === true}
                    busy={busy === "plan"}
                    label={t("计划模式")}
                    onClick={() => run("plan", () => port.setPlanMode(!status?.plan))}
                  />
                </div>
              </Group>
              <Group title={t("这个会话在哪写")}>
                <div className="kv">
                  <span className="k">{t("工作目录")}</span>
                  <span className="v">{status?.cwd ?? "—"}</span>
                </div>
                <p className="note">
                  {t("文件夹在左栏管：底部添加，展开后开新会话。一个会话属于开它的那个文件夹，不会跟着跑到别处。")}
                </p>
                {/* A thing that happens once, not a state to sit in — so it is
                    a button. As an option row it carried a selected look it can
                    never have. */}
                <div className="lrow">
                  <span className="tx">
                    <span className="lb">{t("拉一份隔离副本")}</span>
                    <span className="ds">{t("在 Git worktree 里开一份，改动不落回当前分支")}</span>
                  </span>
                  <button className="act" disabled={!!busy} onClick={() => run("isolate", () => port.isolateWorkspace())}>
                    {t(busy === "isolate" ? "开着…" : "开一份")}
                  </button>
                </div>
              </Group>
            </>
          )}

          {at === "model" && (
            <>
              {/* Every switch on this page goes through run(), so one place to
                  say why one was refused covers all of them. */}
              {failed && (
                <div className="find" data-lvl="warn" role="alert">
                  <span className="t">{t("这一步没做成")}</span>
                  <span className="why">{failed}</span>
                </div>
              )}
              <Group title={t("分工")} now={roles ? t("{n} 个已指派", { n: assigned }) : undefined}
                hint={t("每个位置默认跟着主模型走，只有你明确指派过的才会分出去。换指派跟换主模型一样要重建运行时，有活儿在跑的时候换不了。")}>
                <Roles models={models} roles={roles} main={status?.modelRef} busy={busy}
                  onSet={(role, ref) => run(`role:${role}`, async () => {
                    await port.setRole(role, ref);
                    loadRoles();
                  })} />
              </Group>
              <Group title={t("模型")} now={nav.model} hint={t("切换会带着对话重建运行时；有活儿在跑的时候切不了。标签只写探得到的能力 —— 空着就是没人声明过，不是「不支持」。")}>
                <Models models={models} current={status?.modelRef} busy={busy} protocol={protocol}
                  onPick={(ref) => run(ref, () => port.setModel(ref))} />
              </Group>
              {efforts.length > 0 ? (
                <Group title={t("推理强度")} hint={t("这几档是当前模型的端点真正认的，auto 表示交给它自己的默认。")}>
                  <div className="seg" role="group" aria-label={t("推理强度")}>
                    {efforts.map((e) => (
                      <button key={e} aria-pressed={(status?.effort || "auto") === e}
                        onClick={() => run(e, () => port.setEffort(e))}>
                        {e}
                      </button>
                    ))}
                  </div>
                </Group>
              ) : (
                <Group title={t("推理强度")} hint={t("当前模型没有暴露可调的推理档位，调它不会有任何效果，所以这里不给开关。")} />
              )}
              <Group
                title={t("连接")}
                hint={t("模型从哪里来。添加只问地址和 key —— 协议、模型列表、能不能看图，都去问端点，问不出来的才让你填。")}
              >
                <Providers port={port} onChanged={loadModels} onFailed={setFailed} protocol={protocol}
                  activeKindFor={(a) => kindFor(a.key)}
                  onProtocol={(a, kind) => switchProtocol(a.key, kind)} />
              </Group>
            </>
          )}

          {at === "tools" && (
            <>
              <Group title={t("工具批准")} now={approval} hint={t("这是唯一挡在 agent 和你的文件之间的闸。它拦下来的时候，没有第二个入口能绕过去。")}>
                {/* Four rows of label-and-description was 190px for one choice,
                    and it was the only choice in this pane shaped that way. The
                    description follows the selection instead: what a档 does is
                    read when it is picked, not compared four at a time. */}
                <div className="seg" data-text data-danger={status?.toolApprovalMode === "yolo" ? "" : undefined}
                  role="radiogroup" aria-label={t("工具批准")}>
                  {APPROVALS.map(([id, name]) => (
                    <button key={id} role="radio" aria-checked={status?.toolApprovalMode === id} disabled={!!busy}
                      onClick={() => run(id, () => port.setApprovalMode(id))}>
                      {t(name)}
                    </button>
                  ))}
                </div>
                <p className="note">{t(APPROVALS.find(([id]) => id === status?.toolApprovalMode)?.[2] ?? "")}</p>
              </Group>
              <Group
                title={t("明确的规矩")}
                hint={t("上面那档管的是「问不问你」，这里管的是「哪些根本不许，哪些永远不用问」。改动会重建运行时，有活儿在跑的时候改不了。")}
              >
                <Rules port={port} onChanged={onChanged} />
              </Group>
              <Group
                title={t("沙箱")}
                hint={t("批准之后能碰到多大范围。这一层不靠 agent 自觉：写入范围由工具执行，命令隔离由操作系统执行。")}
              >
                <Sandbox port={port} onChanged={onChanged} />
              </Group>
              <Group
                title={t("命令交给谁执行")}
                hint={t("agent 的每条命令都由这个程序来跑，所以它也决定命令该写成哪一种语法 —— 选错了不是慢，是每条都报错。下面列的是这台机器上真有的，装什么才能选什么。换一个要重建运行时，有活儿在跑的时候换不了。")}
              >
                <ShellPicker port={port} onChanged={onChanged} />
              </Group>
            </>
          )}

          {at === "hooks" && (
            <Group
              title={t("自动化")}
              hint={t("在 agent 干活的前后插进你自己的命令。它们跑在你的机器上，用你的权限 —— 挡得住 agent 的那两个事件在下面会标出来。")}
            >
              <Hooks port={port} onChanged={afterExtChange} />
            </Group>
          )}

          {at === "ext" && (
            <>
              {scope && <ScopeBar scope={scope} scopes={scopes} onPick={setScopeAt} />}
              {failed && (
                <div className="find" data-lvl="warn" role="alert">
                  <span className="t">{t("这一步没做成")}</span>
                  <span className="why">{failed}</span>
                </div>
              )}
              {/* 装完、改完、删完都要过这一步才算数——把它放在包列表上面，
                  因为它管的是整个运行时，不是某一个包。 */}
              <Group
                title={t("运行时")}
                hint={t("改了扩展的代码，或者装、删、开关了插件包之后，用它让改动生效。当前这一轮不受影响，下一轮开始用新的。")}
                action={reload.action}
              >
                {reload.note}
              </Group>
              <Group
                title={t("插件包")}
                now={packages.length ? t("{n} 个", { n: packages.length }) : undefined}
                hint={t("一个包能一次带来技能、命令、自动化钩子和外部服务。装和导入是同一件事：给它一个仓库地址，或者机器上的一个文件夹。")}
                action={
                  addingPkg ? undefined : (
                    <button className="act" onClick={() => setAddingPkg(true)}>
                      {t("添加")}
                    </button>
                  )
                }
              >
                {addingPkg && (
                  <AddPlugin port={port} onClose={() => setAddingPkg(false)} onInstalled={afterExtChange} />
                )}
                {updatingPkg && (
                  <AddPlugin
                    port={port}
                    updating={packages.find((p) => p.name === updatingPkg)}
                    onClose={() => setUpdatingPkg("")}
                    onInstalled={afterExtChange}
                  />
                )}
                <Packages
                  port={port}
                  packages={packages}
                  onChanged={afterExtChange}
                  updating={updatingPkg}
                  onUpdate={setUpdatingPkg}
                />
                {packages.length === 0 && !addingPkg && <div className="empty">{t("还没装插件包。")}</div>}
              </Group>
              {/* Below the packages: what was added by hand. A server the user
                  typed in themselves is not part of anyone's package, and
                  filing it under one would misname where it came from. */}
              <Group
                title={t("外部工具")}
                now={looseMcp.length ? t("{n} 个服务", { n: looseMcp.length }) : undefined}
                hint={t("你自己接进来的 MCP 服务。它给 agent 的能力和内置工具一样真实 —— 列在这里的每一项都能动你的东西。关掉一个会立刻从这一轮的工具表里消失，并且重启后依然是关的。")}
                action={
                  adding || !live ? undefined : (
                    <button className="act" onClick={() => setAdding(true)}>
                      {t("接入服务")}
                    </button>
                  )
                }
              >
                {adding && live && (
                  <AddServer
                    port={port}
                    canProject={!!status?.workspaceRoot}
                    onClose={() => setAdding(false)}
                    onInstalled={afterExtChange}
                  />
                )}
                {looseMcp.map((m) => (
                  <ServerRow key={m.name} m={m} port={port} onDone={afterExtChange} root={scopeAt} live={live} />
                ))}
                {looseMcp.length === 0 && !adding && <div className="empty">{t("没有自己接入的外部服务。")}</div>}
              </Group>
              <Group
                title={t("技能")}
                now={looseSkills.length ? t("{on}/{all} 开着", { on: looseOn, all: looseSkills.length }) : undefined}
                hint={t(
                  implicit
                    ? "工作目录与「我的」里的技能。带 / 的可以自己点名调用；其余的由模型按任务自行判断要不要用。关掉的那些两条路都走不通。改动在下一次新建会话时进入模型的索引。"
                    : "模型自动发现已关闭：现在只有你点名的技能会跑。改动在下一次新建会话时生效。",
                )}
              >
                {looseSkills.map((sk) => (
                  <SkillRow key={sk.name} sk={sk} implicit={implicit} port={port} onDone={afterExtChange} root={scopeAt} onFailed={setFailed} />
                ))}
                {looseSkills.length === 0 && <div className="empty">{t("这个工作目录下没有技能。")}</div>}
              </Group>
            </>
          )}

          {at === "network" && (
            <Group
              title={t("网络")}
              hint={t("模型请求、MCP 的远程服务、网页抓取都走这里。配错了通常表现为聊天时莫名其妙卡住 —— 所以先测一下，它会告诉你断在哪一段。")}
            >
              <Network port={port} />
            </Group>
          )}

          {at === "account" && (
            <Group
              title={t("账号")}
              hint={t("Reasonix 本身不需要账号。它只用在天生要联网的地方：社区发帖、崩溃问题跟进，以后还有技能发布。")}
            >
              <Account port={port} state={acct} reload={reloadAccount} />
            </Group>
          )}

          {at === "versions" && (
            <Group
              title={t("版本")}
              hint={t("装的是哪一版、有没有更新，以及出问题时怎么退回去。回退会固定在你选的版本，不会被自动更新拽回来。")}
            >
              <Versions port={port} />
            </Group>
          )}

          {at === "memory" && (
            <Group
              title={t("记忆")}
              hint={t("它自己记下来的东西 —— 你没配置过，但它会照着做。所以这里按「什么时候会被想起」分，并且标出上一轮真正用上了哪几条。")}
            >
              <Memory port={port} />
            </Group>
          )}

          {at === "storage" && (
            <Group title={t("存储")} hint={t("它把数据写在哪、占了多少。会话和索引会一直长，配置和凭据不会 —— 所以只有前者能搬走，搬迁在重启后生效。")}><Storage port={port} /></Group>
          )}

          {at === "appearance" && (
            <Appearance port={port} theme={theme} onTheme={onTheme} contrast={contrast} onContrast={onContrast} weight={weight} onWeight={onWeight} reloadThemes={reloadThemes} look={look} onLook={onLook} />
          )}

          {at === "advanced" && (
            <Group title={t("还不在这一版里")} hint={t("每一项都需要自己的界面，做半个不如先说清楚它现在在哪。")}>
              {ELSEWHERE.map((x) => (
                <div className="lrow" key={x}>
                  <span className="ds">{x}</span>
                  <span className="sc">{t("旧版桌面端")}</span>
                </div>
              ))}
            </Group>
          )}
          </div>
        </Boundary>
        </div>
      </div>
    </div>
  );
}

function Group({
  title, hint, now, action, children,
}: {
  title: string; hint?: string; now?: string; action?: React.ReactNode; children?: React.ReactNode;
}) {
  return (
    <section className="grp">
      <div className="grp-hd">
        <h2>{title}</h2>
        {now && <span className="now">{now}</span>}
        {action}
      </div>
      {hint && <p className="hint">{hint}</p>}
      {children && <div className="grp-items">{children}</div>}
    </section>
  );
}

