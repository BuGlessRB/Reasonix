import { useMemo, useState, type FormEvent, type KeyboardEvent } from "react";
import { t } from "../i18n";

const START = "reasonix://start";

function normalise(value: string): string | null {
  const raw = value.trim();
  if (!raw) return START;
  if (/^reasonix:\/\//i.test(raw)) return START;
  const local = /^(?:localhost|127(?:\.\d+){3}|\[::1\])(?::\d+)?(?:\/|$)/i.test(raw);
  const candidate = local ? `http://${raw}` : /^[a-z][a-z0-9+.-]*:\/\//i.test(raw) ? raw : `https://${raw}`;
  try {
    const url = new URL(candidate);
    return url.protocol === "http:" || url.protocol === "https:" ? url.href : null;
  } catch {
    return null;
  }
}

function startPage(scheme: "light" | "dark"): string {
  const light = scheme === "light";
  const page = light ? "#f6f7f4" : "#171817";
  const text = light ? "#242622" : "#e9eae5";
  const muted = light ? "#747970" : "#90958d";
  const surface = light ? "#ffffff" : "#1c1d1b";
  const border = light ? "#dfe1db" : "#2c2f2b";
  return `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><style>
    :root{color-scheme:${scheme};font-family:Inter,"Noto Sans SC",system-ui,sans-serif;background:${page};color:${text}}
    *{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;padding:32px;background:${page}}
    main{width:min(520px,100%);text-align:center}.mark{width:42px;height:42px;margin:0 auto 18px;display:grid;place-items:center;border:1px solid ${border};border-radius:12px;background:${surface};color:#7188c4;font-size:22px}
    h1{margin:0 0 8px;font-size:18px;font-weight:620;letter-spacing:-.02em}p{margin:0;color:${muted};font-size:13px;line-height:1.7}
    .hint{margin-top:22px;padding:11px 14px;border:1px solid ${border};border-radius:9px;background:${surface};color:${muted};font-size:12px;text-align:left}
  </style></head><body><main><div class="mark">◎</div><h1>Reasonix Browser</h1><p>${t("在上方地址栏输入网址，即可在当前任务旁浏览资料。")}</p><div class="hint">${t("浏览内容不会替换或中断当前会话。部分网站禁止嵌入时，可使用右上角的外部打开。")}</div></main></body></html>`;
}

interface Props {
  onClose: () => void;
  onExternal: (url: string) => void;
  scheme: "light" | "dark";
}

export function BrowserPanel({ onClose, onExternal, scheme }: Props) {
  const [history, setHistory] = useState([START]);
  const [index, setIndex] = useState(0);
  const [input, setInput] = useState(START);
  const [reload, setReload] = useState(0);
  const [invalid, setInvalid] = useState(false);
  const [loading, setLoading] = useState(false);
  const current = history[index] ?? START;
  const internal = current === START;
  const srcDoc = useMemo(() => startPage(scheme), [scheme]);

  const visit = (value: string) => {
    const next = normalise(value);
    if (!next) {
      setInvalid(true);
      return;
    }
    setInvalid(false);
    setLoading(next !== START);
    setInput(next);
    if (next === current) {
      setReload((value) => value + 1);
      return;
    }
    const nextHistory = history.slice(0, index + 1).concat(next);
    setHistory(nextHistory);
    setIndex(nextHistory.length - 1);
  };

  const move = (next: number) => {
    const value = history[next];
    if (!value) return;
    setIndex(next);
    setInput(value);
    setInvalid(false);
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    visit(input);
  };

  // Some desktop webviews consume Enter before a form submit bubbles back to
  // React. Commit the address at the field as well; IME confirmation must not
  // navigate while the user is still composing Chinese text.
  const commitAddress = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== "Enter" || event.nativeEvent.isComposing) return;
    event.preventDefault();
    event.stopPropagation();
    visit(event.currentTarget.value);
  };

  return (
    <aside className="side studio-browser" aria-label={t("内置 Browser")}>
      <header className="studio-browser-head">
        <span><b>Browser</b><small>{t("内置浏览")}</small></span>
        <button type="button" data-action="browser.close" onClick={onClose} aria-label={t("关闭 Browser")} title={t("关闭")}>
          <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m4 4 8 8M12 4l-8 8" /></svg>
        </button>
      </header>
      <form className="studio-browser-toolbar" onSubmit={submit} data-action-submit="browser.navigate">
        <button type="button" data-action="browser.back" onClick={() => move(index - 1)} disabled={index === 0} aria-label={t("后退")} title={t("后退")}>
          <svg viewBox="0 0 16 16" aria-hidden="true"><path d="m9.5 3-5 5 5 5M5 8h7" /></svg>
        </button>
        <button type="button" data-action="browser.reload" onClick={() => setReload((value) => value + 1)} aria-label={t("刷新")} title={t("刷新")}>
          <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M12.7 6A5 5 0 1 0 13 9.5M12.7 6V2.8M12.7 6H9.5" /></svg>
        </button>
        <label className="studio-browser-address" data-invalid={invalid ? "" : undefined}>
          <svg viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="5.3" /><path d="M2.8 8h10.4M8 2.7c1.5 1.5 2.2 3.2 2.2 5.3S9.5 11.8 8 13.3C6.5 11.8 5.8 10.1 5.8 8S6.5 4.2 8 2.7Z" /></svg>
          <input data-action-change="browser.address" data-action-keydown="browser.navigate" value={input} onKeyDown={commitAddress} onChange={(event) => { setInput(event.target.value); setInvalid(false); }} aria-label={t("网页地址")} spellCheck={false} autoCapitalize="off" />
        </label>
        <button type="submit" className="studio-browser-go" aria-label={t("打开网页")}>{t("打开")}</button>
        <button type="button" data-action="browser.external" onClick={() => !internal && onExternal(current)} disabled={internal} aria-label={t("在外部浏览器打开")} title={t("在外部浏览器打开")}>
          <svg viewBox="0 0 16 16" aria-hidden="true"><path d="M9 3h4v4M13 3 7.5 8.5" /><path d="M7 4H3v9h9V9" /></svg>
        </button>
      </form>
      {invalid && <p className="studio-browser-error" role="alert">{t("请输入有效的 http 或 https 地址")}</p>}
      <iframe
        key={`${current}:${reload}`}
        className="studio-browser-frame"
        title={t("Reasonix 内置 Browser")}
        src={internal ? undefined : current}
        srcDoc={internal ? srcDoc : undefined}
        sandbox="allow-scripts allow-forms allow-popups"
        referrerPolicy="no-referrer"
        onLoad={() => setLoading(false)}
        onError={() => setLoading(false)}
      />
      <footer className="studio-browser-note" data-loading={loading ? "" : undefined}>
        {loading ? t("正在打开网页…") : t("页面空白通常表示网站禁止嵌入，可使用右上角按钮在外部浏览器打开。")}
      </footer>
    </aside>
  );
}
