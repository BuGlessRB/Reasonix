import { useCallback, useEffect, useState } from "react";
import { t } from "../i18n";
import type { ProviderCheck, ProviderEdit, ProviderEntry, ProviderProbe } from "../port/port";
import { KIND_LABEL, accountKey, disambiguate, hostOf, vendorLabel } from "./vendors";

// A connection is an account, not a config row. One endpoint answering two
// protocols is two rows in the file and one service to the person paying for it,
// so the rows group by host and the protocol becomes a switch on the account.
//
// Adding one is still two questions — where and with what key — because the rest
// is knowable by asking the endpoint.

type Port = {
  providers(): Promise<ProviderEntry[]>;
  probeProvider(baseUrl: string, apiKey: string): Promise<ProviderProbe>;
  saveProvider(draft: {
    name: string; kind: string; baseUrl: string; apiKey: string; models: string[];
    default: string; authHeader: boolean; noProxy: boolean; effort: string; vision: string[];
  }): Promise<void>;
  removeProvider(name: string): Promise<void>;
  checkProvider(name: string): Promise<ProviderCheck>;
  editProvider(edit: ProviderEdit): Promise<void>;
  setProviderWebSearch(name: string, on: boolean): Promise<void>;
  setProviderThinking(name: string, on: boolean): Promise<void>;
};

// A name for the config table, derived from the host so the user does not have
// to invent one. "api.moonshot.cn/v1" becomes "moonshot".
export function nameFrom(baseUrl: string): string {
  try {
    const host = new URL(baseUrl).hostname.replace(/^(www|api|open)\./, "");
    return host.split(".")[0].replace(/[^a-zA-Z0-9._-]/g, "-") || "custom";
  } catch {
    return "custom";
  }
}

// One account: every configured entry that answers on the same host.
interface Account {
  key: string;
  label: string;
  host: string;
  // The config entry's own name, shown only when one host holds two accounts.
  hint: string;
  byKind: Record<string, ProviderEntry>;
  kinds: string[];
}

function groupAccounts(list: ProviderEntry[]): Account[] {
  const out = new Map<string, Account>();
  for (const p of list) {
    const host = hostOf(p.baseUrl);
    const key = accountKey(host, p.keyEnv);
    let a = out.get(key);
    if (!a) {
      a = { key, label: vendorLabel(host), host, hint: p.name, byKind: {}, kinds: [] };
      out.set(key, a);
    }
    const kind = p.kind || "openai";
    if (!a.byKind[kind]) {
      a.byKind[kind] = p;
      a.kinds.push(kind);
    }
  }
  return disambiguate([...out.values()]);
}

interface ProvidersProps {
  port: Port;
  onChanged: () => void;
  // Which protocol each account is showing, and how to change it. The model
  // list reads the same map, so switching here re-lists the models below.
  protocol: Record<string, string>;
  onProtocol: (account: Account, kind: string) => void;
  activeKindFor: (account: Account) => string;
}

export function Providers({ port, onChanged, protocol, onProtocol, activeKindFor }: ProvidersProps) {
  const [list, setList] = useState<ProviderEntry[] | null>(null);
  const [adding, setAdding] = useState(false);
  const [busy, setBusy] = useState("");

  const reload = useCallback(() => {
    port.providers().then(setList).catch(() => setList([]));
  }, [port]);
  useEffect(reload, [reload]);

  const remove = async (name: string) => {
    setBusy(name);
    try {
      await port.removeProvider(name);
      reload();
      onChanged();
    } finally {
      setBusy("");
    }
  };

  if (list === null) return <p className="acct-note">{t("正在读取…")}</p>;

  return (
    <>
      <div className="vlist">
        {groupAccounts(list).map((a) => (
          <Conn key={a.key} a={a} port={port} busy={busy} setBusy={setBusy}
            kind={protocol[a.key] ?? activeKindFor(a)}
            onProtocol={(k) => onProtocol(a, k)}
            onRemove={remove}
            onEdited={() => { reload(); onChanged(); }} />
        ))}
        {list.length === 0 && <div className="empty">{t("还没有配置任何模型来源。")}</div>}
      </div>
      {adding ? (
        <AddProvider
          port={port}
          taken={list.map((p) => p.name)}
          known={list}
          onDone={() => {
            setAdding(false);
            reload();
            onChanged();
          }}
          onCancel={() => setAdding(false)}
        />
      ) : (
        <button className="lnk" onClick={() => setAdding(true)}>
          {t("添加模型来源")}
        </button>
      )}
    </>
  );
}

// One account. The protocol is a switch on it rather than a fact on a row,
// because both entries are the same key at the same host; 测一下 is what turns
// "which protocol did we record" back into a finding when the endpoint moved.
function Conn({
  a, port, busy, setBusy, kind, onProtocol, onRemove, onEdited,
}: {
  a: Account; port: Port; busy: string; setBusy: (b: string) => void;
  kind: string; onProtocol: (kind: string) => void; onRemove: (name: string) => void;
  onEdited: () => void;
}) {
  const [found, setFound] = useState<ProviderCheck | null>(null);
  const [editing, setEditing] = useState(false);
  const entry = a.byKind[kind] ?? a.byKind[a.kinds[0]];
  const checking = busy === `check:${entry.name}`;
  const inUse = a.kinds.some((k) => a.byKind[k].inUse);

  const setSearch = async (on: boolean) => {
    setBusy(`search:${entry.name}`);
    try {
      await port.setProviderWebSearch(entry.name, on);
      onEdited();
    } finally {
      setBusy("");
    }
  };

  const setThinking = async (on: boolean) => {
    setBusy(`thinking:${entry.name}`);
    try {
      await port.setProviderThinking(entry.name, on);
      onEdited();
    } finally {
      setBusy("");
    }
  };

  const check = async () => {
    setBusy(`check:${entry.name}`);
    setFound(null);
    try {
      setFound(await port.checkProvider(entry.name));
    } catch (e) {
      setFound({ ok: false, error: e instanceof Error ? e.message : String(e) });
    } finally {
      setBusy("");
    }
  };

  const models = entry.models.length;
  return (
    <>
      <div className="vrow" data-on={inUse ? "" : undefined}>
        <span className="nm">{a.label}</span>
        <span className="ds">
          {a.host}
          {models > 0 ? ` · ${t("{n} 个模型", { n: models })}` : ""}
          {t(entry.hasKey ? "" : " · 缺 key")}
        </span>
        <span className="sc">{t(inUse ? "正在用" : "")}</span>
        {/* Hover-reveal is right for 删除; a diagnostic nobody can find is not
            a diagnostic, so this one stays on the row. */}
        <button className="sa lnk" data-keep onClick={() => setEditing((v) => !v)} disabled={busy !== ""}>
          {t(editing ? "收起" : "编辑")}
        </button>
        <button className="sa lnk" data-keep onClick={check} disabled={busy !== ""}>
          {t(checking ? "测试中…" : "测一下")}
        </button>
        {!entry.inUse && (
          <button className="sa lnk" onClick={() => onRemove(entry.name)} disabled={busy !== ""}>
            {t("删除")}
          </button>
        )}
      </div>
      {a.kinds.length > 1 && (
        <div className="vway">
          <span className="lb">{t("接入方式")}</span>
          <div className="seg" role="group" aria-label={t("{name} 的接入方式", { name: a.label })}>
            {a.kinds.map((k) => (
              <button key={k} aria-pressed={k === kind} disabled={busy !== ""} onClick={() => onProtocol(k)}>
                {t(KIND_LABEL[k] ?? k)}
                {/* A door that carries a capability the other lacks has to say
                    so on itself: switching is otherwise a silent downgrade. */}
                {a.byKind[k].canWebSearch && <i className="perk">{t("联网搜索")}</i>}
              </button>
            ))}
          </div>
          <span className="why">
            {t(a.kinds.some((k) => a.byKind[k].canWebSearch) && !entry.canWebSearch ? "同一个账号的两扇门。这一扇没有联网搜索 —— 那是协议的差别，不是设置。" : "同一个账号的两扇门。换一扇，下面的模型跟着换。")}
          </span>
        </div>
      )}
      {entry.canWebSearch && (
        <div className="vway">
          <span className="lb">{t("联网搜索")}</span>
          <div className="seg" role="group" aria-label={t("{name} 的联网搜索", { name: a.label })}>
            {[true, false].map((on) => (
              <button key={String(on)} aria-pressed={entry.webSearch === on} disabled={busy !== ""}
                onClick={() => setSearch(on)}>
                {t(on ? "开" : "关")}
              </button>
            ))}
          </div>
          <span className="why">{t("端点自己执行的搜索，不占本地工具。")}</span>
        </div>
      )}
      {entry.canSetThinking && (
        <div className="vway">
          <span className="lb">{t("思考参数")}</span>
          <div className="seg" role="group" aria-label={t("{name} 的思考参数", { name: a.label })}>
            {[true, false].map((on) => (
              <button key={String(on)} aria-pressed={(entry.sendsThinking ?? true) === on} disabled={busy !== ""}
                onClick={() => setThinking(on)}>
                {t(on ? "自动" : "不发送")}
              </button>
            ))}
          </div>
          <span className="why">
            {t(entry.sendsThinking === false ? "只发普通聊天参数。模型自己该怎么想还怎么想，只是这边不再指定深度。" : "有的中转站不认 thinking 字段，会整个请求拒掉。真遇上了就切「不发送」。")}
          </span>
        </div>
      )}
      {editing && (
        <EditConn
          entry={entry}
          port={port}
          busy={busy}
          setBusy={setBusy}
          onDone={() => {
            setEditing(false);
            onEdited();
          }}
        />
      )}
      {found && (
        <div className="find" data-lvl={found.ok ? "ok" : "warn"} role="status">
          <span className="t">
            {found.ok
              ? `${t("连上了")} · ${t(KIND_LABEL[found.kind ?? ""] ?? found.kind ?? "")} · ${t("{n} 个模型", { n: found.models?.length ?? 0 })}`
              : t("连不上")}
          </span>
          <span className="why">
            {!found.ok && found.error}
            {found.ok && found.kind !== entry.kind &&
              t("记的是 {had}，但它答的是 {got}。", { had: t(KIND_LABEL[entry.kind] ?? entry.kind), got: t(KIND_LABEL[found.kind ?? ""] ?? found.kind ?? "") })}
            {found.ok && found.kind === entry.kind && "key 有效，协议也对得上。"}
            {found.ok && found.noProxy && " 走代理连不上、直连可以。"}
          </span>
        </div>
      )}
    </>
  );
}

// Editing a saved source. Only what this form owns is sent: the entry keeps its
// prices, effort vocabularies and everything else the panel cannot show.
function EditConn({
  entry, port, busy, setBusy, onDone,
}: {
  entry: ProviderEntry; port: Port; busy: string; setBusy: (b: string) => void; onDone: () => void;
}) {
  const [baseUrl, setBaseUrl] = useState(entry.baseUrl);
  const [apiKey, setApiKey] = useState("");
  const [models, setModels] = useState<string[]>(entry.models);
  const [picked, setPicked] = useState<string[]>(entry.models);
  const [vision, setVision] = useState<string[]>(entry.visionModels ?? []);
  const [def, setDef] = useState(entry.default || entry.models[0] || "");
  const [err, setErr] = useState("");
  const [more, setMore] = useState(false);
  const [win, setWin] = useState(entry.contextWindow ? String(entry.contextWindow) : "");
  const [heads, setHeads] = useState(headerLines(entry.headers));
  const [extra, setExtra] = useState(entry.extraBody ? JSON.stringify(entry.extraBody, null, 2) : "");
  const saving = busy === `edit:${entry.name}`;
  const extraBad = extra.trim() !== "" && parseExtraBody(extra) === null;

  const toggle = (list: string[], set: (v: string[]) => void, m: string) =>
    set(list.includes(m) ? list.filter((x) => x !== m) : [...list, m]);

  // Re-asking the endpoint is how a source that gained models catches up; the
  // ticks the user already made survive it.
  // A blank key field means "keep the stored one", so re-probing has to go
  // through the saved source. Sending the empty field instead probes as a
  // provider with no credential at all, which fails before it reaches the host.
  const refetch = async () => {
    setBusy(`edit:${entry.name}`);
    setErr("");
    try {
      const found = apiKey.trim()
        ? (await port.probeProvider(baseUrl.trim(), apiKey.trim())).models
        : (await port.checkProvider(entry.name)).models ?? [];
      if (found.length === 0) throw new Error("这个端点没报出任何聊天模型");
      setModels([...new Set([...found, ...picked])]);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  };

  const save = async () => {
    setBusy(`edit:${entry.name}`);
    setErr("");
    try {
      await port.editProvider({
        name: entry.name,
        baseUrl: baseUrl.trim(),
        apiKey: apiKey.trim(),
        models: picked,
        default: picked.includes(def) ? def : picked[0] ?? "",
        vision: vision.filter((m) => picked.includes(m)),
        contextWindow: Number(win.replace(/\D/g, "")) || 0,
        headers: parseHeaders(heads),
        extraBody: parseExtraBody(extra) ?? {},
      });
      onDone();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="addp" data-edit>
      <div className="fields">
        <label className="grow full">
          <span>{t("接口地址")}</span>
          <input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} spellCheck={false} />
        </label>
        <label className="grow full">
          <span>{t("API Key（留空就不动它）")}</span>
          <input type="password" value={apiKey} placeholder="········"
            onChange={(e) => setApiKey(e.target.value)} spellCheck={false} />
        </label>
      </div>

      <div className="mlist">
        <span className="mlb">
          模型（{picked.length}/{models.length}）·{" "}
          {t(entry.canSetVision === false ? "这个端点不接受图片输入，勾了也不会生效" : "勾「读图」的才会收到图片")}
        </span>
        {models.map((m) => (
          <div className="mline" key={m} data-off={picked.includes(m) ? undefined : ""}>
            <button className="tick" role="checkbox" aria-checked={picked.includes(m)}
              aria-label={`选用 ${m}`} onClick={() => toggle(picked, setPicked, m)}>
              <i />
            </button>
            <span className="nm">{m}</span>
            <button className="vtag" aria-pressed={vision.includes(m)}
              disabled={!picked.includes(m) || entry.canSetVision === false}
              title={entry.canSetVision === false ? "内核不给这个端点发图片，改这里不会有效果" : undefined}
              onClick={() => toggle(vision, setVision, m)}>
              {t("读图")}
            </button>
            <button className="dtag" aria-pressed={def === m} disabled={!picked.includes(m)}
              onClick={() => setDef(m)}>
              {t("默认")}
            </button>
          </div>
        ))}
      </div>

      {/* Folded, and worth folding: these three are the ones no probe can
          answer, and most endpoints need none of them. */}
      <button className="more" aria-expanded={more} onClick={() => setMore((v) => !v)}>
        {t(more ? "收起" : "端点要求的额外设置")}
        <span className="c">{compatSummary(win, heads, extra)}</span>
      </button>

      {more && (
        <div className="fields compat">
          <label className="grow full">
            <span>{t("上下文窗口（tokens）")}</span>
            <input
              inputMode="numeric"
              value={win}
              placeholder={t("留空 = 用内置的已知值；0 = 这个来源不自动压缩")}
              onChange={(e) => setWin(e.target.value.replace(/\D/g, ""))}
            />
            <i className="tip">
              {t("填模型文档写的上下文上限，不是最大输出。填小了会一直压缩，填大了会在真到上限时被端点拒绝。")}
            </i>
          </label>
          <label className="grow full">
            <span>{t("额外请求头")}</span>
            <textarea
              rows={3}
              value={heads}
              spellCheck={false}
              placeholder={"HTTP-Referer: https://example.com\nX-Title: Reasonix"}
              onChange={(e) => setHeads(e.target.value)}
            />
            <i className="tip">{t("一行一个 名字: 值。中转站常要它来认站点；密钥仍然走上面那栏。")}</i>
          </label>
          <label className="grow full">
            <span>{t("额外请求体")}</span>
            <textarea
              rows={4}
              value={extra}
              spellCheck={false}
              placeholder={'{\n  "enable_thinking": true\n}'}
              onChange={(e) => setExtra(e.target.value)}
              aria-invalid={extraBad || undefined}
            />
            <i className="tip">
              {t("会并进请求体的顶层。model、messages、tools、stream 这些仍由内核说了算，写了也不生效。")}
            </i>
          </label>
          {extraBad && <div className="why">{t("这段不是合法的 JSON 对象，保存会被拒绝。")}</div>}
        </div>
      )}

      {err && (
        <div className="find" data-lvl="warn">
          <span className="t">{t("没保存成功")}</span>
          <span className="why">{err}</span>
        </div>
      )}

      <div className="acts">
        <button className="act" data-primary onClick={save} disabled={busy !== "" || picked.length === 0 || extraBad}>
          {t(saving ? "保存中…" : "保存")}
        </button>
        <button className="act" onClick={refetch} disabled={busy !== ""}
          title={t("重新问这个端点要一次模型列表 —— 它上新或下架模型之后用")}>
          {t("重新问一次有哪些模型")}
        </button>
        <button className="act" onClick={onDone} disabled={busy !== ""}>{t("取消")}</button>
      </div>
    </div>
  );
}

function AddProvider({
  port, taken, known, onDone, onCancel,
}: {
  port: Port; taken: string[]; known: ProviderEntry[]; onDone: () => void; onCancel: () => void;
}) {
  const [baseUrl, setBaseUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [probe, setProbe] = useState<ProviderProbe | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  // Everything below is editable after the probe, because every one of these
  // is something the endpoint could not tell us for certain.
  const [name, setName] = useState("");
  const [kind, setKind] = useState("");
  const [picked, setPicked] = useState<string[]>([]);

  // A source already at this host changes what a blank key means: another door
  // onto that account rather than an account with no credential.
  const sibling = known.find((p) => hostOf(p.baseUrl) === hostOf(baseUrl) && baseUrl.trim() !== "");

  const connect = async () => {
    setBusy(true);
    setErr("");
    try {
      const got = await port.probeProvider(baseUrl.trim(), apiKey.trim());
      setProbe(got);
      setKind(got.kind);
      setPicked(got.models.slice(0, 8));
      setName(uniqueName(nameFrom(baseUrl), taken));
    } catch (e) {
      setProbe(null);
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  };

  const save = async () => {
    if (!probe) return;
    setBusy(true);
    setErr("");
    try {
      await port.saveProvider({
        name: name.trim(),
        kind,
        baseUrl: baseUrl.trim(),
        apiKey: apiKey.trim(),
        models: picked,
        default: picked[0] ?? "",
        authHeader: probe.authHeader,
        noProxy: probe.noProxy,
        effort: "",
        vision: probe.vision.filter((m) => picked.includes(m)),
      });
      onDone();
    } catch (e) {
      setErr(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  };

  const toggle = (m: string) =>
    setPicked((cur) => (cur.includes(m) ? cur.filter((x) => x !== m) : [...cur, m]));

  return (
    <div className="addp">
      <div className="fields">
        <label className="grow full">
          <span>{t("接口地址")}</span>
          <input
            value={baseUrl}
            placeholder="https://api.moonshot.cn/v1"
            onChange={(e) => setBaseUrl(e.target.value)}
            spellCheck={false}
          />
        </label>
        <label className="grow full">
          <span>API Key{t(sibling ? "（留空就用现有那个来源的 key）" : "")}</span>
          <input type="password" value={apiKey} placeholder={sibling ? "········" : ""}
            onChange={(e) => setApiKey(e.target.value)} spellCheck={false} />
        </label>
        {sibling && (
          <p className="acct-note">
            这个地址上已经有「{vendorLabel(hostOf(sibling.baseUrl))}」了。留空 key
            {t("就是给它再开一扇门，两条会并成同一个来源、由「接入方式」切换；填了 key就是这台机器上的另一个账号，各算各的。")}
          </p>
        )}
      </div>

      <div className="acts">
        <button className="act" data-primary onClick={connect} disabled={busy || baseUrl.trim() === ""}>
          {t(busy && !probe ? "连接中…" : "连一下试试")}
        </button>
        <button className="act" onClick={onCancel} disabled={busy}>
          {t("取消")}
        </button>
      </div>

      {err && (
        <div className="find" data-lvl="warn">
          <span className="t">{t("连不上")}</span>
          <span className="why">{err}</span>
        </div>
      )}

      {probe && (
        <>
          {/* The heading says these are guesses, so no row has to repeat it. */}
          <p className="acct-note">{t("探到了下面这些。都是猜的，不对就改。")}</p>

          <div className="fields">
            <label className="grow">
              <span>{t("名字")}</span>
              <input value={name} onChange={(e) => setName(e.target.value)} spellCheck={false} />
            </label>
            <label className="grow">
              <span>{t("接入方式")}</span>
              <select value={kind} onChange={(e) => setKind(e.target.value)}>
                <option value="openai">{t("OpenAI 兼容")}</option>
                <option value="anthropic">{t("Anthropic 兼容")}</option>
              </select>
            </label>
          </div>
          {probe.ambiguous && (
            <p className="acct-note">
              {t("两种接入方式的模型列表它都答得上来，光看列表分不出来 —— 聊天入口通常不在同一个路径下，选错了聊天会报错。要两条都用就再添加一次、选另一个。")}
            </p>
          )}
          {probe.noProxy && (
            <p className="acct-note">{t("走代理连不上、直连可以，已经记成「这个来源不走代理」。")}</p>
          )}

          <div className="fields">
            <span className="mlb">模型（{picked.length}/{probe.models.length}）</span>
            <div className="mpick">
              {probe.models.map((m) => (
                <button
                  key={m}
                  className="chip"
                  aria-pressed={picked.includes(m)}
                  onClick={() => toggle(m)}
                >
                  {m}
                  {t(probe.vision.includes(m) ? " ·图" : "")}
                </button>
              ))}
            </div>
          </div>

          <div className="acts">
            <button className="act" data-primary onClick={save} disabled={busy || picked.length === 0 || name.trim() === ""}>
              {t(busy ? "保存中…" : "添加")}
            </button>
          </div>
        </>
      )}
    </div>
  );
}

// A second provider from the same vendor must not overwrite the first.
function uniqueName(base: string, taken: string[]): string {
  if (!taken.includes(base)) return base;
  for (let i = 2; i < 100; i++) {
    if (!taken.includes(`${base}-${i}`)) return `${base}-${i}`;
  }
  return base;
}

// The three compatibility fields move between a config object and the text the
// user types. Headers are one "name: value" per line because that is how the
// gateway's own documentation writes them.
function headerLines(headers?: Record<string, string>): string {
  return Object.entries(headers ?? {})
    .map(([k, v]) => `${k}: ${v}`)
    .join("\n");
}

function parseHeaders(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const at = line.indexOf(":");
    if (at <= 0) continue;
    const name = line.slice(0, at).trim();
    const value = line.slice(at + 1).trim();
    if (name && value) out[name] = value;
  }
  return out;
}

// null means "typed but not valid JSON yet", which is different from an empty
// object — the save button reads the difference rather than sending garbage.
function parseExtraBody(text: string): Record<string, unknown> | null {
  if (!text.trim()) return {};
  try {
    const parsed: unknown = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return null;
    return parsed as Record<string, unknown>;
  } catch {
    return null;
  }
}

function compatSummary(win: string, heads: string, extra: string): string {
  const parts: string[] = [];
  if (win.trim()) parts.push(win === "0" ? t("不压缩") : `${Number(win) / 1000}k`);
  const headCount = Object.keys(parseHeaders(heads)).length;
  if (headCount) parts.push(t("{n} 个头", { n: headCount }));
  const body = parseExtraBody(extra);
  if (body && Object.keys(body).length) parts.push(t("有请求体"));
  return parts.join(" · ");
}
