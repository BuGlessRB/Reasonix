import { useCallback, useEffect, useRef, useState } from "react";
import type { AccountState, AgentPort, ApprovalMode, McpEntry, ModelEntry, Preset, RoleAssignments, SessionStatus, SkillEntry } from "../port/port";
import { arrowTabs } from "./tablist";
import { WindowControls } from "./WindowControls";
import { AddServer } from "./AddServer";
import { Hooks } from "./Hooks";
import { Network } from "./Network";
import { Account } from "./Account";
import { Providers } from "./Providers";
import { Models, activeKind, groupVendors } from "./Models";
import { Roles } from "./Roles";
import { Boundary } from "./Boundary";
import { Versions } from "./Versions";
import { Memory } from "./Memory";

const PRESETS: [Preset, string, string][] = [
  ["light", "轻量", "简单就直接做，只做针对性验证，高风险才叫独立复核"],
  ["balanced", "均衡", "按复杂度自适应。这是默认档"],
  ["delivery", "交付", "完整验收证据，中高风险改动强制独立复核"],
];

const APPROVALS: [ApprovalMode, string, string][] = [
  ["ask", "询问", "每次动手前问你"],
  ["auto", "自动", "低风险自己过，写操作仍然问"],
  ["dontAsk", "不再问", "这一类记住，本会话内不再问"],
  ["yolo", "全放行", "不问了。只在你完全信任这个工作区时用"],
];

const THEMES: [string, string][] = [
  ["auto", "跟随系统"],
  ["light", "浅色"],
  ["dark", "深色"],
];

type Section = "session" | "model" | "tools" | "hooks" | "ext" | "network" | "memory" | "account" | "versions" | "appearance" | "advanced";

// What still lives in the old desktop app. Bots and theme packs are not on the
// roadmap, so they are not promises to keep here either. Signing in and reading
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
  ["account", "账号"],
  ["versions", "版本"],
  ["appearance", "外观"],
  ["advanced", "高级"],
].filter(([id]) => id !== "advanced" || ELSEWHERE.length > 0) as [Section, string][];

const SCOPE: Record<string, string> = { project: "项目", custom: "自定义", global: "我的", builtin: "内置" };

// The user's question is "is it there and does it work", so the state is the
// label. A failed server keeps its error on the row that names it.
const NET_MODE: Record<string, string> = { auto: "跟随系统", env: "环境变量", custom: "手动", off: "直连" };

const MCP_STATE: Record<string, string> = {
  ready: "已连接",
  connecting: "连接中",
  failed: "连不上",
  disabled: "已关闭",
  idle: "未连接",
};

interface Props {
  port: AgentPort;
  status: SessionStatus | null;
  theme: string;
  onTheme: (t: string) => void;
  onClose: () => void;
  onChanged: () => void;
  at?: string;
  account: AccountState | null;
  reloadAccount: () => void;
}

export function Settings({ port, status, theme, onTheme, onClose, onChanged, at: opened, account: acct, reloadAccount }: Props) {
  const [at, setAt] = useState<Section>((opened as Section) || "session");
  const [models, setModels] = useState<ModelEntry[]>([]);
  const [roles, setRoles] = useState<RoleAssignments | null>(null);
  const [protocol, setProtocol] = useState<Record<string, string>>({});
  const [mcp, setMcp] = useState<McpEntry[]>([]);
  const [skills, setSkills] = useState<SkillEntry[]>([]);
  const [implicit, setImplicit] = useState(true);
  const [busy, setBusy] = useState("");
  const [adding, setAdding] = useState(false);
  const [hookCount, setHookCount] = useState(0);
  const [netMode, setNetMode] = useState("");
  const [memCount, setMemCount] = useState(0);
  const root = useRef<HTMLDivElement>(null);

  const reloadExt = useCallback(() => {
    port.mcp().then(setMcp).catch(() => setMcp([]));
    port.hooks().then((c) => setHookCount(c.hooks.length)).catch(() => setHookCount(0));
    port.network().then((n) => setNetMode(NET_MODE[n.mode] ?? n.mode)).catch(() => setNetMode(""));
    port.memories().then((c) => setMemCount(c.memories.length)).catch(() => setMemCount(0));
    port
      .skills()
      .then((c) => {
        setSkills(c.skills);
        setImplicit(c.implicit);
      })
      .catch(() => setSkills([]));
  }, [port]);

  // An extension switch moves the metrics rail too, so the change has to leave
  // this pane as well as refresh it.
  const afterExtChange = useCallback(() => {
    reloadExt();
    onChanged();
  }, [reloadExt, onChanged]);

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

  const run = async (what: string, fn: () => Promise<void>) => {
    setBusy(what);
    try {
      await fn();
      onChanged();
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
  // Switching the door an account is reached through is a real switch when that
  // account is the one running: the same model on the other protocol is a
  // different endpoint, so it has to go through setModel like any other change.
  const switchProtocol = (key: string, kind: string) => {
    setProtocol((p) => ({ ...p, [key]: kind }));
    const v = vendors.find((x) => x.key === key);
    const running = v && Object.values(v.byKind).flat().some((m) => m.ref === status?.modelRef);
    if (!v || !running) return;
    const here = models.find((m) => m.ref === status?.modelRef);
    const same = (v.byKind[kind] ?? []).find((m) => m.model === here?.model);
    if (same && same.ref !== status?.modelRef) run(same.ref, () => port.setModel(same.ref));
  };
  const efforts = models.find((m) => m.ref === status?.modelRef)?.efforts ?? [];
  const assigned = roles ? Object.values(roles).filter(Boolean).length : 0;
  const preset = PRESETS.find(([id]) => id === status?.preset)?.[1] ?? "—";
  const approval = APPROVALS.find(([id]) => id === status?.toolApprovalMode)?.[1] ?? "—";
  const broken = mcp.filter((m) => m.state === "failed").length;
  const skillsOn = skills.filter((s) => s.enabled).length;
  // The table of contents is also the status board: the value that matters for
  // each section rides on its own row, so the risky one is legible from here.
  const nav: Record<Section, string> = {
    session: status?.plan ? "计划模式" : preset,
    model: status?.modelRef?.split("/").pop() ?? "—",
    tools: approval,
    hooks: hookCount ? `${hookCount} 条` : "关",
    ext: broken ? `${broken} 个异常` : `${mcp.length + skillsOn}`,
    network: netMode,
    memory: memCount ? `${memCount} 条` : "",
    account: acct === null ? "" : acct.signedIn ? (acct.user?.label ?? "已登录") : "未登录",
    versions: "",
    appearance: THEMES.find(([id]) => id === theme)?.[1] ?? "",
    advanced: ELSEWHERE.length ? `${ELSEWHERE.length}` : "",
  };
  const danger = (id: Section) =>
    (id === "tools" && status?.toolApprovalMode === "yolo") || (id === "ext" && broken > 0);

  return (
    <div className="prefs" ref={root} tabIndex={-1}>
      <div className="prefs-hd">
        <button className="back" onClick={onClose}>
          ‹ 返回工作台
        </button>
        <span className="ttl">设置</span>
        <span className="esc">esc</span>
      </div>

      <div className="prefs-body">
        <nav className="prefs-nav" role="tablist" aria-label="设置分类" onKeyDown={arrowTabs}>
          {NAV.map(([id, name]) => (
            <button key={id} id={`prefs-${id}`} role="tab" aria-selected={at === id} onClick={() => setAt(id)}>
              {name}
              <span className="nv" data-danger={danger(id) ? "" : undefined}>
                {nav[id]}
              </span>
            </button>
          ))}
        </nav>

        <div className="prefs-main" role="tabpanel" aria-labelledby={`prefs-${at}`} data-sec={at}>
          <Boundary
            fallback={
              <div className="find" data-lvl="err" role="alert">
                <span className="t">这个设置分区出错了</span>
                <span className="why">其它分区和你的会话不受影响；关掉设置再打开可重试。</span>
              </div>
            }
          >
          <div className="prefs-col">
          {at === "session" && (
            <>
              <Group title="执行设定" hint="管的是规划深度、验证广度、独立复核频率 —— 不是省钱档位。切档立刻生效，不重建运行时。">
                {PRESETS.map(([id, name, desc]) => (
                  <Row key={id} on={status?.preset === id} busy={busy === id} label={name} desc={desc}
                    onClick={() => run(id, () => port.setPreset(id))} />
                ))}
              </Group>
              <Group title="计划模式" hint="开着的时候拿不到写权限：这不是提示词里的约定，是没给这个能力。">
                <Row on={status?.plan === true} label="开" desc="只读加出计划，你批准后核心自己关掉它"
                  onClick={() => run("plan-on", () => port.setPlanMode(true))} />
                <Row on={status?.plan === false} label="关" desc="正常执行"
                  onClick={() => run("plan-off", () => port.setPlanMode(false))} />
              </Group>
              <Group title="这个会话在哪写">
                <div className="kv">
                  <span className="k">工作目录</span>
                  <span className="v">{status?.cwd ?? "—"}</span>
                </div>
                <p className="note">
                  在顶栏点项目名换目录。换目录会整个重建运行时，当前对话留在原来那个
                  项目里，不跟过去。
                </p>
              </Group>
            </>
          )}

          {at === "model" && (
            <>
              <Group title="分工" now={roles ? `${assigned} 个已指派` : undefined}
                hint="每个位置默认跟着主模型走，只有你明确指派过的才会分出去。换指派跟换主模型一样要重建运行时，有活儿在跑的时候换不了。">
                <Roles models={models} roles={roles} main={status?.modelRef} busy={busy}
                  onSet={(role, ref) => run(`role:${role}`, async () => {
                    await port.setRole(role, ref);
                    loadRoles();
                  })} />
              </Group>
              <Group title="模型" now={nav.model} hint="切换会带着对话重建运行时；有活儿在跑的时候切不了。标签只写探得到的能力 —— 空着就是没人声明过，不是「不支持」。">
                <Models models={models} current={status?.modelRef} busy={busy} protocol={protocol}
                  onPick={(ref) => run(ref, () => port.setModel(ref))} />
              </Group>
              {efforts.length > 0 ? (
                <Group title="推理强度" hint="这几档是当前模型的端点真正认的，auto 表示交给它自己的默认。">
                  <div className="seg" role="group" aria-label="推理强度">
                    {efforts.map((e) => (
                      <button key={e} aria-pressed={(status?.effort || "auto") === e}
                        onClick={() => run(e, () => port.setEffort(e))}>
                        {e}
                      </button>
                    ))}
                  </div>
                </Group>
              ) : (
                <Group title="推理强度" hint="当前模型没有暴露可调的推理档位，调它不会有任何效果，所以这里不给开关。" />
              )}
              <Group
                title="连接"
                hint="模型从哪里来。添加只问地址和 key —— 协议、模型列表、能不能看图，都去问端点，问不出来的才让你填。"
              >
                <Providers port={port} onChanged={loadModels} protocol={protocol}
                  activeKindFor={(a) => kindFor(a.key)}
                  onProtocol={(a, kind) => switchProtocol(a.key, kind)} />
              </Group>
            </>
          )}

          {at === "tools" && (
            <Group title="工具批准" hint="这是唯一挡在 agent 和你的文件之间的闸。它拦下来的时候，没有第二个入口能绕过去。">
              {APPROVALS.map(([id, name, desc]) => (
                <Row key={id} on={status?.toolApprovalMode === id} busy={busy === id} danger={id === "yolo"}
                  label={name} desc={desc} onClick={() => run(id, () => port.setApprovalMode(id))} />
              ))}
            </Group>
          )}

          {at === "hooks" && (
            <Group
              title="自动化"
              hint="在 agent 干活的前后插进你自己的命令。它们跑在你的机器上，用你的权限 —— 挡得住 agent 的那两个事件在下面会标出来。"
            >
              <Hooks port={port} onChanged={afterExtChange} />
            </Group>
          )}

          {at === "ext" && (
            <>
              <Group
                title="外部工具"
                now={mcp.length ? `${mcp.length} 个服务` : undefined}
                hint="通过 MCP 接进来的服务。它给 agent 的能力和内置工具一样真实 —— 列在这里的每一项都能动你的东西。关掉一个会立刻从这一轮的工具表里消失，并且重启后依然是关的。"
                action={
                  adding ? undefined : (
                    <button className="act" onClick={() => setAdding(true)}>
                      接入服务
                    </button>
                  )
                }
              >
                {adding && (
                  <AddServer
                    port={port}
                    canProject={!!status?.workspaceRoot}
                    onClose={() => setAdding(false)}
                    onInstalled={afterExtChange}
                  />
                )}
                {mcp.map((m) => (
                  <Server key={m.name} m={m} port={port} onDone={afterExtChange} />
                ))}
                {mcp.length === 0 && !adding && <div className="empty">没有接入任何外部服务。</div>}
              </Group>
              <Group
                title="技能"
                now={skills.length ? `${skillsOn}/${skills.length} 开着` : undefined}
                hint={
                  implicit
                    ? "带 / 的可以自己点名调用；其余的由模型按任务自行判断要不要用。关掉的那些两条路都走不通。改动在下一次新建会话时进入模型的索引。"
                    : "模型自动发现已关闭：现在只有你点名的技能会跑。改动在下一次新建会话时生效。"
                }
              >
                {skills.map((sk) => (
                  <SkillRow key={sk.name} sk={sk} implicit={implicit} port={port} onDone={afterExtChange} />
                ))}
                {skills.length === 0 && <div className="empty">这个工作目录下没有技能。</div>}
              </Group>
            </>
          )}

          {at === "network" && (
            <Group
              title="网络"
              hint="模型请求、MCP 的远程服务、网页抓取都走这里。配错了通常表现为聊天时莫名其妙卡住 —— 所以先测一下，它会告诉你断在哪一段。"
            >
              <Network port={port} />
            </Group>
          )}

          {at === "account" && (
            <Group
              title="账号"
              hint="Reasonix 本身不需要账号。它只用在天生要联网的地方：社区发帖、崩溃问题跟进，以后还有技能发布。"
            >
              <Account port={port} state={acct} reload={reloadAccount} />
            </Group>
          )}

          {at === "versions" && (
            <Group
              title="版本"
              hint="装的是哪一版、有没有更新，以及出问题时怎么退回去。回退会固定在你选的版本，不会被自动更新拽回来。"
            >
              <Versions port={port} />
            </Group>
          )}

          {at === "memory" && (
            <Group
              title="记忆"
              hint="它自己记下来的东西 —— 你没配置过，但它会照着做。所以这里按「什么时候会被想起」分，并且标出上一轮真正用上了哪几条。"
            >
              <Memory port={port} />
            </Group>
          )}

          {at === "appearance" && (
            <Group title="主题" hint="跟随系统时，系统切换会立刻反映；手动选过就固定住。">
              {/* 三个没有说明文字的枚举值，跟推理强度是同一种选择，就该长成同一个控件 */}
              <div className="seg" data-text role="group" aria-label="主题">
                {THEMES.map(([id, name]) => (
                  <button key={id} aria-pressed={theme === id} onClick={() => onTheme(id)}>
                    {name}
                  </button>
                ))}
              </div>
            </Group>
          )}

          {at === "advanced" && (
            <Group title="还不在这一版里" hint="每一项都需要自己的界面，做半个不如先说清楚它现在在哪。">
              {ELSEWHERE.map((x) => (
                <div className="lrow" key={x}>
                  <span className="ds">{x}</span>
                  <span className="sc">旧版桌面端</span>
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

// The tool list is the long half and the least urgent, so it folds behind its
// own count — the same idiom a long file read uses in the transcript.
function Server({ m, port, onDone }: { m: McpEntry; port: AgentPort; onDone: () => void }) {
  const [busy, setBusy] = useState("");
  const [failed, setFailed] = useState("");
  const [confirming, setConfirming] = useState(false);
  // 401/403 is not a broken server, it is a server that stopped trusting this
  // machine — retrying without saying so sends the user around the same loop.
  const auth = /\b(401|403|unauthorized|forbidden|auth)/i.test(m.error ?? failed);
  const meta = [MCP_STATE[m.state] ?? m.state, m.transport, m.source].filter(Boolean).join(" · ");

  const run = async (what: string, fn: () => Promise<unknown>) => {
    setBusy(what);
    setFailed("");
    try {
      const r = (await fn()) as { error?: string } | void;
      if (r && typeof r === "object" && r.error) setFailed(r.error);
    } catch (e) {
      setFailed(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
      onDone();
    }
  };

  const actions = (
    <span className="acts">
      {m.enabled && m.state !== "ready" && (
        <button className="act" disabled={!!busy} onClick={() => void run("retry", () => port.reconnectMcp(m.name))}>
          {busy === "retry" ? "连接中…" : auth ? "重新授权" : "重连"}
        </button>
      )}
      {/* Removal is the one action here that cannot be undone by clicking again,
          so it asks — and the question names the file it is about to edit. */}
      <button className="act ghost" aria-label={`移除 ${m.name}`} disabled={!!busy} onClick={() => setConfirming(true)}>
        移除
      </button>
      <Switch
        on={m.enabled}
        busy={busy === "toggle"}
        label={`${m.enabled ? "关闭" : "启用"} ${m.name}`}
        onClick={() => void run("toggle", () => port.setMcpEnabled(m.name, !m.enabled))}
      />
    </span>
  );

  const confirm = confirming && (
    <div className="confirm">
      <span className="q">
        把 {m.name} 从{m.source ? ` ${m.source} ` : "配置"}里删掉？只是想暂时不用的话，关掉开关就够了。
      </span>
      <button className="act" onClick={() => setConfirming(false)}>
        算了
      </button>
      <button
        className="act danger"
        disabled={busy === "remove"}
        onClick={() =>
          void run("remove", async () => {
            const r = await port.removeMcp(m.name);
            setConfirming(false);
            // A lower-precedence declaration with the same name may have taken
            // over; saying so beats a list that looks like the delete failed.
            if (r.stillConfigured) setFailed("同名的另一处声明现在生效了，这一行不会消失。");
          })
        }
      >
        {busy === "remove" ? "移除中…" : "移除"}
      </button>
    </div>
  );

  const head = (
    <>
      <i className="pip" />
      <span className="nm">{m.name}</span>
      {m.toolNames?.length ? <span className="fold">{m.tools} 个工具</span> : null}
      <span className="meta">{meta}</span>
      {actions}
    </>
  );
  const why = m.error || failed;
  if (!m.toolNames?.length) {
    return (
      <div className="srv" data-st={m.state}>
        <div className="srv-hd">{head}</div>
        {why && <div className="why">{why}</div>}
        {confirm}
      </div>
    );
  }
  return (
    // Asking to remove has to open the row: the confirmation lives inside the
    // fold, and what the server contributes is worth seeing before dropping it.
    <details className="srv" data-st={m.state} open={confirming || undefined}>
      <summary>{head}</summary>
      {confirm}
      <div className="peek">
        {m.toolNames.map((t) => (
          <div className="row" key={t}>
            <span className="d">·</span>
            <span>{t}</span>
          </div>
        ))}
      </div>
    </details>
  );
}

// How a skill can start is the thing the slash list cannot say: a slash name
// means you can call it, "auto" means the model may start it on its own, and
// those are separate permissions. Both is the norm, so only the row that is
// missing one says anything — a badge on every row is a badge on none.
function triggerNote(sk: SkillEntry, implicit: boolean): string {
  if (!sk.enabled) return "";
  const auto = !sk.manual && implicit;
  if (sk.slashName && auto) return "";
  if (sk.slashName) return "只能点名";
  if (auto) return "只能模型自选";
  return "调不到";
}

function SkillRow({
  sk, implicit, port, onDone,
}: {
  sk: SkillEntry; implicit: boolean; port: AgentPort; onDone: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const note = triggerNote(sk, implicit);
  const toggle = async () => {
    setBusy(true);
    try {
      await port.setSkillEnabled(sk.name, !sk.enabled);
    } finally {
      setBusy(false);
      onDone();
    }
  };
  return (
    <div className="skrow" data-off={sk.enabled ? undefined : ""}>
      <span className="nm">{sk.slashName ? "/" + sk.slashName : sk.name}</span>
      <span className="ds">{sk.description || "没有写说明"}</span>
      <span className="how">{note && <i className={note === "调不到" ? "w none" : "w"}>{note}</i>}</span>
      <span className="face">
        {sk.subagent && <i className="sa">子代理</i>}
        {sk.readOnly && <i className="ro">只读</i>}
      </span>
      <span className="sc" title={sk.path}>
        {sk.plugin || SCOPE[sk.scope ?? ""] || sk.scope}
      </span>
      <Switch on={sk.enabled} busy={busy} label={`${sk.enabled ? "关闭" : "启用"} ${sk.name}`} onClick={toggle} />
    </div>
  );
}

function Switch({
  on, busy, label, onClick,
}: {
  on: boolean; busy?: boolean; label: string; onClick: () => void;
}) {
  return (
    <button className="sw" role="switch" aria-checked={on} aria-label={label} disabled={busy} onClick={onClick}>
      <i />
    </button>
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

function Row({
  on, busy, danger, label, desc, onClick,
}: {
  on?: boolean; busy?: boolean; danger?: boolean; label: string; desc?: string; onClick: () => void;
}) {
  return (
    <button className="prow" data-on={on ? "" : undefined} data-danger={danger ? "" : undefined}
      onClick={onClick} disabled={busy}>
      <span className="mark" />
      <span className="tx">
        <span className="lb">{label}</span>
        {desc && <span className="ds">{desc}</span>}
      </span>
    </button>
  );
}
