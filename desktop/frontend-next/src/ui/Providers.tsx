import { useCallback, useEffect, useState } from "react";
import type { ProviderEntry, ProviderProbe } from "../port/port";

// Adding a model is two questions — where and with what key — because the rest
// is knowable by asking the endpoint. Everything the probe reports is shown as
// a guess with a way to change it: a model list proves which protocols an
// endpoint answers, never which one it actually speaks.

type Port = {
  providers(): Promise<ProviderEntry[]>;
  probeProvider(baseUrl: string, apiKey: string): Promise<ProviderProbe>;
  saveProvider(draft: {
    name: string; kind: string; baseUrl: string; apiKey: string; models: string[];
    default: string; authHeader: boolean; noProxy: boolean; effort: string; vision: string[];
  }): Promise<void>;
  removeProvider(name: string): Promise<void>;
};

const KIND_LABEL: Record<string, string> = { openai: "OpenAI 兼容", anthropic: "Anthropic 兼容", responses: "Responses" };

// A name for the config table, derived from the host so the user does not have
// to invent one. "api.moonshot.cn/v1" becomes "moonshot".
function nameFrom(baseUrl: string): string {
  try {
    const host = new URL(baseUrl).hostname.replace(/^(www|api|open)\./, "");
    return host.split(".")[0].replace(/[^a-zA-Z0-9._-]/g, "-") || "custom";
  } catch {
    return "custom";
  }
}

export function Providers({ port, onChanged }: { port: Port; onChanged: () => void }) {
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

  if (list === null) return <p className="acct-note">正在读取…</p>;

  return (
    <>
      <div className="vlist">
        {list.map((p) => (
          <div key={p.name} className="vrow" data-on={p.inUse ? "" : undefined}>
            <span className="nm">{p.name}</span>
            <span className="ds">
              {KIND_LABEL[p.kind] || p.kind}
              {p.models.length > 1 ? ` · ${p.models.length} 个模型` : ""}
              {p.hasKey ? "" : " · 缺 key"}
            </span>
            <span className="sc">{p.inUse ? "正在用" : p.preset ? "内置" : ""}</span>
            {!p.inUse && (
              <button className="sa lnk" onClick={() => remove(p.name)} disabled={busy !== ""}>
                删除
              </button>
            )}
          </div>
        ))}
        {list.length === 0 && <div className="empty">还没有配置任何模型来源。</div>}
      </div>
      {adding ? (
        <AddProvider
          port={port}
          taken={list.map((p) => p.name)}
          onDone={() => {
            setAdding(false);
            reload();
            onChanged();
          }}
          onCancel={() => setAdding(false)}
        />
      ) : (
        <button className="lnk" onClick={() => setAdding(true)}>
          添加模型来源
        </button>
      )}
    </>
  );
}

function AddProvider({
  port, taken, onDone, onCancel,
}: {
  port: Port; taken: string[]; onDone: () => void; onCancel: () => void;
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
          <span>接口地址</span>
          <input
            value={baseUrl}
            placeholder="https://api.moonshot.cn/v1"
            onChange={(e) => setBaseUrl(e.target.value)}
            spellCheck={false}
          />
        </label>
        <label className="grow full">
          <span>API Key</span>
          <input type="password" value={apiKey} onChange={(e) => setApiKey(e.target.value)} spellCheck={false} />
        </label>
      </div>

      <div className="acts">
        <button className="act" data-primary onClick={connect} disabled={busy || baseUrl.trim() === ""}>
          {busy && !probe ? "连接中…" : "连一下试试"}
        </button>
        <button className="act" onClick={onCancel} disabled={busy}>
          取消
        </button>
      </div>

      {err && (
        <div className="find" data-lvl="warn">
          <span className="t">连不上</span>
          <span className="why">{err}</span>
        </div>
      )}

      {probe && (
        <>
          {/* The heading says these are guesses, so no row has to repeat it. */}
          <p className="acct-note">探到了下面这些。都是猜的，不对就改。</p>

          <div className="fields">
            <label className="grow">
              <span>名字</span>
              <input value={name} onChange={(e) => setName(e.target.value)} spellCheck={false} />
            </label>
            <label className="grow">
              <span>协议</span>
              <select value={kind} onChange={(e) => setKind(e.target.value)}>
                <option value="openai">OpenAI 兼容</option>
                <option value="anthropic">Anthropic 兼容</option>
              </select>
            </label>
          </div>
          {probe.ambiguous && (
            <p className="acct-note">
              这个端点两种协议都答得上来，光看模型列表分不出来。选错了聊天会报错，那时候回来换另一个。
            </p>
          )}
          {probe.noProxy && (
            <p className="acct-note">走代理连不上、直连可以，已经记成「这个来源不走代理」。</p>
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
                  {probe.vision.includes(m) ? " ·图" : ""}
                </button>
              ))}
            </div>
          </div>

          <div className="acts">
            <button className="act" data-primary onClick={save} disabled={busy || picked.length === 0 || name.trim() === ""}>
              {busy ? "保存中…" : "添加"}
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
