import { useCallback, useEffect, useRef, useState } from "react";
import type { AgentPort, ApprovalMode, McpEntry, ModelEntry, Preset, SessionStatus, SkillEntry } from "../port/port";
import { arrowTabs } from "./tablist";

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

const EFFORTS = ["auto", "low", "medium", "high", "xhigh", "max"];

const THEMES: [string, string][] = [
  ["auto", "跟随系统"],
  ["light", "浅色"],
  ["dark", "深色"],
];

type Section = "session" | "model" | "tools" | "ext" | "appearance" | "advanced";

const NAV: [Section, string][] = [
  ["session", "会话"],
  ["model", "模型"],
  ["tools", "工具与权限"],
  ["ext", "扩展"],
  ["appearance", "外观"],
  ["advanced", "高级"],
];

const SCOPE: Record<string, string> = { project: "项目", custom: "自定义", global: "我的", builtin: "内置" };

// Adding and removing servers rewrites config files and needs its own surface;
// everything else about a server is actionable from here.
const ELSEWHERE = ["新增 / 删除 MCP 服务器", "Hooks", "记忆", "主题包", "机器人接入", "网络与代理", "账号与更新"];

// The user's question is "is it there and does it work", so the state is the
// label. A failed server keeps its error on the row that names it.
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
}

export function Settings({ port, status, theme, onTheme, onClose, onChanged }: Props) {
  const [at, setAt] = useState<Section>("session");
  const [models, setModels] = useState<ModelEntry[]>([]);
  const [mcp, setMcp] = useState<McpEntry[]>([]);
  const [skills, setSkills] = useState<SkillEntry[]>([]);
  const [implicit, setImplicit] = useState(true);
  const [busy, setBusy] = useState("");
  const root = useRef<HTMLDivElement>(null);

  const reloadExt = useCallback(() => {
    port.mcp().then(setMcp).catch(() => setMcp([]));
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

  useEffect(() => {
    port.models().then(setModels).catch(() => setModels([]));
    reloadExt();
    root.current?.focus();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [port, onClose, reloadExt]);

  const run = async (what: string, fn: () => Promise<void>) => {
    setBusy(what);
    try {
      await fn();
      onChanged();
    } finally {
      setBusy("");
    }
  };

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
    ext: broken ? `${broken} 个异常` : `${mcp.length + skillsOn}`,
    appearance: THEMES.find(([id]) => id === theme)?.[1] ?? "",
    advanced: "",
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
              <Group title="模型" now={nav.model} hint="切换会带着对话重建运行时；有活儿在跑的时候切不了。">
                {models.map((m) => (
                  <Row key={m.ref} on={m.ref === status?.modelRef} busy={busy === m.ref}
                    label={m.model} desc={m.provider} onClick={() => run(m.ref, () => port.setModel(m.ref))} />
                ))}
                {models.length === 0 && <div className="empty">读不到模型列表。</div>}
              </Group>
              <Group title="推理强度" hint="可选档位随 provider 能力变化，auto 表示交给 provider 默认。">
                <div className="seg" role="group" aria-label="推理强度">
                  {EFFORTS.map((e) => (
                    <button key={e} aria-pressed={(status?.effort || "auto") === e}
                      onClick={() => run(e, () => port.setEffort(e))}>
                      {e}
                    </button>
                  ))}
                </div>
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

          {at === "ext" && (
            <>
              <Group
                title="外部工具"
                now={mcp.length ? `${mcp.length} 个服务` : undefined}
                hint="通过 MCP 接进来的服务。它给 agent 的能力和内置工具一样真实 —— 列在这里的每一项都能动你的东西。关掉一个会立刻从这一轮的工具表里消失，并且重启后依然是关的。"
              >
                {mcp.map((m) => (
                  <Server key={m.name} m={m} port={port} onDone={afterExtChange} />
                ))}
                {mcp.length === 0 && <div className="empty">没有接入任何外部服务。</div>}
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
      <Switch
        on={m.enabled}
        busy={busy === "toggle"}
        label={`${m.enabled ? "关闭" : "启用"} ${m.name}`}
        onClick={() => void run("toggle", () => port.setMcpEnabled(m.name, !m.enabled))}
      />
    </span>
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
      </div>
    );
  }
  return (
    <details className="srv" data-st={m.state}>
      <summary>{head}</summary>
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
  title, hint, now, children,
}: {
  title: string; hint?: string; now?: string; children: React.ReactNode;
}) {
  return (
    <section className="grp">
      <div className="grp-hd">
        <h2>{title}</h2>
        {now && <span className="now">{now}</span>}
      </div>
      {hint && <p className="hint">{hint}</p>}
      <div className="grp-items">{children}</div>
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
