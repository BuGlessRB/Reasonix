import { useEffect, useState } from "react";
import { t } from "../i18n";
import type { AgentPort, SandboxSettings } from "../port/port";
import { Switch } from "./Switch";

// Two questions, in the order they matter: how far a write reaches, and whether
// the command that makes it runs jailed. The first is always in force; the
// second needs an OS sandbox and says so where there is none.
export function Sandbox({ port, onChanged }: { port: AgentPort; onChanged: () => void }) {
  const [box, setBox] = useState<SandboxSettings | null>(null);
  const [root, setRoot] = useState("");
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    port
      .sandbox()
      .then((s) => {
        setBox(s);
        setRoot(s.workspaceRoot);
      })
      .catch(() => setBox(null));
  }, [port]);

  if (!box) return <div className="empty">{t("读不到沙箱配置。")}</div>;

  const save = async (what: string, next: SandboxSettings) => {
    setBusy(what);
    setError("");
    try {
      const saved = await port.saveSandbox(next);
      setBox(saved);
      setRoot(saved.workspaceRoot);
      onChanged();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  };

  const pick = async () => {
    const path = await port.pickFolder().catch(() => null);
    if (path) void save("root", { ...box, workspaceRoot: path });
  };

  return (
    <div className="box">
      {box.shadowedBy && (
        <div className="find" data-lvl="warn" role="status">
          <span className="t">{t("这个项目自带一份沙箱配置")}</span>
          <span className="why">
            {t("{path} 里也写了 sandbox，实际生效的是它。", { path: box.shadowedBy })}
          </span>
        </div>
      )}

      <div className="sec">
        <h3>{t("能写到哪")}</h3>
        <p className="note">
          {t("批准过的写操作也只能落在这些目录里。这不是提示词里的约定，是文件工具拿不到别处的句柄。")}
        </p>
        <div className="fields">
          <label className="grow">
            <span>{t("工作区根目录")}</span>
            <input
              value={root}
              placeholder={t("留空 = 会话所在的目录")}
              onChange={(e) => setRoot(e.target.value)}
              onBlur={() => root !== box.workspaceRoot && void save("root", { ...box, workspaceRoot: root })}
              onKeyDown={(e) => e.key === "Enter" && void save("root", { ...box, workspaceRoot: root })}
            />
          </label>
          <button className="act" disabled={!!busy} onClick={() => void pick()}>
            {t("选文件夹")}
          </button>
        </div>

        <div className="extra">
          <div className="sublb">{t("另外还能写")}</div>
          {box.allowWrite.map((p) => (
            <div className="prule" key={p}>
              <code>{p}</code>
              <button
                className="act ghost"
                disabled={!!busy}
                aria-label={t("不再允许写 {path}", { path: p })}
                onClick={() => void save("extra", { ...box, allowWrite: box.allowWrite.filter((x) => x !== p) })}
              >
                {t("删掉")}
              </button>
            </div>
          ))}
          <div className="radd">
            <input
              value={draft}
              placeholder={t("再开一个可写目录，例如 /tmp/scratch")}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key !== "Enter" || !draft.trim()) return;
                void save("extra", { ...box, allowWrite: [...box.allowWrite, draft.trim()] });
                setDraft("");
              }}
            />
            <button
              className="act"
              disabled={!!busy || !draft.trim()}
              onClick={() => {
                void save("extra", { ...box, allowWrite: [...box.allowWrite, draft.trim()] });
                setDraft("");
              }}
            >
              {t("加上")}
            </button>
          </div>
        </div>

        {/* The expansion, not the spelling: an empty root above is still a real
            directory down here, and ${VAR} in a path is answered rather than
            echoed back. */}
        <div className="kv">
          <span className="k">{t("实际可写")}</span>
          <span className="v">
            {box.effectiveWriteRoots.length ? box.effectiveWriteRoots.map((r) => <code key={r}>{r}</code>) : "—"}
          </span>
        </div>
      </div>

      <div className="sec">
        <h3>{t("命令怎么跑")}</h3>
        {box.available ? (
          <p className="note">
            {t("关进沙箱之后，命令连想写别处都做不到 —— 上面那份可写清单会由操作系统来执行，而不是由 agent 自觉遵守。")}
          </p>
        ) : (
          <div className="find" data-lvl="warn" role="status">
            <span className="t">{t("这台机器没有可用的 OS 沙箱")}</span>
            <span className="why">{box.why || t("命令只能不受限地运行；上面的可写范围仍然由工具自己执行。")}</span>
          </div>
        )}
        <div className="seg" data-text role="radiogroup" aria-label={t("命令怎么跑")}>
          {[
            ["enforce", "关进沙箱"],
            ["off", "不受限"],
          ].map(([id, label]) => (
            <button
              key={id}
              role="radio"
              aria-checked={(box.bash || "off") === id}
              disabled={!!busy || (id === "enforce" && !box.available)}
              onClick={() => void save("bash", { ...box, bash: id })}
            >
              {t(label)}
            </button>
          ))}
        </div>
        {box.bash === "enforce" && (
          <div className="lrow">
            <span className="tx">
              <span className="lb">{t("沙箱里允许联网")}</span>
              <span className="ds">{t("关掉之后装依赖、拉仓库都会失败 —— 这正是它的用途")}</span>
            </span>
            <Switch
              on={box.network}
              busy={busy === "net"}
              label={t("沙箱里允许联网")}
              onClick={() => void save("net", { ...box, network: !box.network })}
            />
          </div>
        )}
      </div>
      {error && <div className="why">{error}</div>}
    </div>
  );
}
