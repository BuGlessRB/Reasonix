import { useCallback, useEffect, useState } from "react";
import type { AgentPort, ShellOption, ShellSettings } from "../port/port";

// The product name, not the executable: "powershell.exe" is what the file is
// called, "Windows PowerShell" is what the user installed.
const LABEL: Record<string, string> = {
  bash: "bash",
  "git-bash": "Git Bash",
  pwsh: "PowerShell 7",
  powershell: "Windows PowerShell",
};

// What changes for the person reading it. Only the 5.1 line is a warning: it is
// the one interpreter that refuses syntax the model writes by habit.
const NOTE: Record<string, string> = {
  pwsh: "认得 && 和 ||，但语法仍然是 PowerShell，不是 bash",
  powershell: "不认 && 和 ||，链式命令得拆成两条",
};

const label = (o: ShellOption) => LABEL[o.name] ?? o.name;

// A typed-in path still has to say which family it belongs to, because that is
// what decides how the command is spelled — and the file name is the only clue
// on offer before it has been run.
const kindOf = (path: string) => (/pwsh/i.test(path) ? "pwsh" : /powershell/i.test(path) ? "powershell" : "bash");

export function Shell({ port, onChanged }: { port: AgentPort; onChanged?: () => void }) {
  const [s, setS] = useState<ShellSettings | null>(null);
  const [busy, setBusy] = useState("");
  const [failed, setFailed] = useState("");
  const [custom, setCustom] = useState("");

  const load = useCallback(() => {
    port
      .shell()
      .then((v) => {
        setS(v);
        setCustom(v.path ?? "");
      })
      .catch(() => setS(null));
  }, [port]);

  useEffect(load, [load]);

  if (!s) return <div className="empty">读不到 shell 配置。</div>;

  const options = s.options ?? [];
  // Two bashes on one machine are two different programs, so picking one has to
  // pin its path. With only one, the name alone survives a PATH that moves.
  const pin = (o: ShellOption) => options.filter((x) => x.prefer === o.prefer).length > 1;
  const save = async (what: string, prefer: string, path: string) => {
    setBusy(what);
    setFailed("");
    try {
      const next = await port.saveShell(prefer, path);
      setS(next);
      setCustom(next.path ?? "");
      onChanged?.();
    } catch (e) {
      setFailed(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  };

  const auto = s.prefer === "auto" && !s.path;
  const picked = (o: ShellOption) => !auto && s.effective.path === o.path;
  // A Windows host without bash is the case worth naming: the model's POSIX
  // habits are wrong there, and one install is what changes that.
  const noBash = s.platform === "windows" && !options.some((o) => o.prefer === "bash");

  return (
    <div className="shell">
      <div className="kv">
        <span className="k">当前生效</span>
        <span className="v">
          {label(s.effective)}
          {s.effective.version ? ` ${s.effective.version}` : ""} · {s.effective.path || "—"}
        </span>
      </div>

      <div className="grp-items">
        <button className="prow" data-on={auto ? "" : undefined} disabled={!!busy}
          onClick={() => void save("auto", "auto", "")}>
          <span className="mark" />
          <span className="tx">
            <span className="lb">自动</span>
            <span className="ds">
              自己找，优先真 bash。这台机器上会选到 {label(s.auto)}
            </span>
          </span>
        </button>
        {options.map((o) => (
          <button key={o.path} className="prow" data-on={picked(o) ? "" : undefined} disabled={!!busy}
            onClick={() => void save(o.path, o.prefer, pin(o) ? o.path : "")}>
            <span className="mark" />
            <span className="tx">
              <span className="lb">
                {label(o)}
                {o.version && <i className="ver">{o.version}</i>}
              </span>
              <span className="ds">
                <code>{o.path}</code>
                {NOTE[o.name] && <em className="cav">{NOTE[o.name]}</em>}
              </span>
            </span>
          </button>
        ))}
      </div>

      {noBash && (
        <p className="note">
          这台机器上没有 bash，所以命令只能按 PowerShell 写。装一个 Git for Windows
          就会多出 Git Bash 这一项 —— WSL 里的那个不算，它看到的是 /mnt 下的另一套路径，
          够不着这个工作目录。
        </p>
      )}

      <details className="pinpath">
        <summary>
          <span className="fold">指定一个可执行文件</span>
        </summary>
        <p className="note">
          自己编的 bash、MSYS2、装在别处的 pwsh 都填这里。保存前会真的拿它跑一条命令，
          跑不起来就不会写进配置。
        </p>
        <div className="fields">
          <label className="grow full">
            <span>可执行文件路径</span>
            <input
              value={custom}
              placeholder={s.auto.path}
              onChange={(e) => setCustom(e.target.value)}
            />
          </label>
        </div>
        <div className="acts">
          {s.path && (
            <button className="act" disabled={!!busy} onClick={() => void save("clear", s.prefer, "")}>
              取消指定
            </button>
          )}
          <button className="act" data-primary disabled={!!busy || !custom.trim()}
            onClick={() => void save("custom", kindOf(custom), custom.trim())}>
            {busy === "custom" ? "验证中…" : "保存这个路径"}
          </button>
        </div>
      </details>

      {failed && (
        <div className="find" data-lvl="warn" role="alert">
          <span className="t">没换成</span>
          <span className="why">{failed}</span>
        </div>
      )}
    </div>
  );
}
