import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { t } from "../i18n";
import type { AgentPort, ApprovalMode, ModelEntry, SessionStatus, Attachment } from "../port/port";
import { Picker } from "./Menu";
import { modelMenu } from "./modelmenu";
import { CompletionMenu, useCompletion } from "./Completion";
import { useIme } from "./ime";

const APPROVALS: [ApprovalMode, string, string][] = [
  ["ask", "询问", "每次动手前问你。"],
  ["auto", "自动", "低风险自己过，写操作仍然问。"],
  ["dontAsk", "不再问", "这一类记住，本会话不再问。"],
  ["yolo", "全放行", "不问了。只在你完全信任这个工作区时用。"],
];

const EFFORTS = ["auto", "low", "medium", "high", "xhigh", "max"];

interface Props {
  port: AgentPort;
  status: SessionStatus | null;
  running: boolean;
  onSubmit: (text: string) => void;
  onChanged: () => void;
  onError: (e: unknown) => void;
}

export function Composer({ port, status, running, onSubmit, onChanged, onError }: Props) {
  const [text, setText] = useState("");
  // The caret decides which token is being completed, so it is state here
  // rather than something read off the element when a menu happens to open.
  const [caret, setCaret] = useState(0);
  const [shots, setShots] = useState<Attachment[]>([]);
  const [over, setOver] = useState(false);
  const picker = useRef<HTMLInputElement>(null);
  const [models, setModels] = useState<ModelEntry[]>([]);
  const [switching, setSwitching] = useState(false);
  const box = useRef<HTMLTextAreaElement>(null);
  // Set only when a completion moved the caret: the browser puts it at the end
  // of a programmatic value, which is wrong for anything accepted mid-line.
  const pending = useRef<number | null>(null);

  useEffect(() => {
    port.models().then(setModels).catch(() => setModels([]));
  }, [port]);

  const type = (next: string, at: number) => {
    setText(next);
    setCaret(at);
  };

  const menu = useCompletion(port, text, caret, (next, at) => {
    pending.current = at;
    type(next, at);
    box.current?.focus();
  });
  const ime = useIme();

  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    if (pending.current !== null) {
      el.setSelectionRange(pending.current, pending.current);
      pending.current = null;
    }
    // CSS caps the top at five lines; the element still has to be told to grow.
    // The floor is not decoration: under an interface zoom, scrollHeight is not
    // in the same units the height we write back is, and the two engines do not
    // round it the same way. Writing a smaller number than one line squeezes the
    // box shut — an empty composer with both scrollbars showing and nowhere to
    // type. One line is the least it can ever legitimately be.
    const line = parseFloat(getComputedStyle(el).lineHeight) || 22;
    el.style.height = "auto";
    el.style.height = `${Math.max(line, el.scrollHeight)}px`;
  }, [text]);

  // Attachments ride into the turn as path references, exactly as they do from
  // the CLI — the host saved the bytes, the turn parser resolves the token.
  const send = () => {
    const v = text.trim();
    if (!v && shots.length === 0) return;
    const line = [...shots.map((a) => a.ref), v].filter(Boolean).join(" ");
    type("", 0);
    setShots([]);
    onSubmit(line);
  };

  const grab = (files: Blob[]) => {
    const images = files.filter((f) => f.type.startsWith("image/"));
    if (images.length === 0) return;
    Promise.all(images.map((f) => port.attach(f)))
      .then((added) => setShots((prev) => [...prev, ...added]))
      .catch(onError);
  };

  const apv = status?.toolApprovalMode ?? "ask";
  const modelLb = status?.modelRef?.split("/").pop() ?? status?.label ?? "—";
  // Every one of these rebuilds the runtime kernel-side (~0.4s on a real
  // session). Without a pending state the click reads as a dead control.
  const change = (p: Promise<void>) => {
    setSwitching(true);
    void p.then(onChanged).catch(onError).finally(() => setSwitching(false));
  };

  return (
    <>
      {menu.open && (
        <CompletionMenu
          items={menu.completion.items}
          active={menu.active}
          kind={menu.completion.kind}
          query={menu.completion.query ?? ""}
          onPick={menu.accept}
          onHover={menu.hover}
        />
      )}
      {shots.length > 0 && (
        <div className="shots">
          {shots.map((a) => (
            <span className="shot" key={a.ref} title={a.path}>
              <span className="nm">{a.path.split("/").pop()}</span>
              <button aria-label={t("移除这张图")} onClick={() => setShots((p) => p.filter((x) => x !== a))}>
                ×
              </button>
            </span>
          ))}
          {/* The kernel keeps the image either way, but a text-only model never
              sees it — say so here rather than letting the paste vanish. */}
          {status?.vision === false && <span className="warn">{t("当前模型不读图 · 将交给能读图的子代理")}</span>}
        </div>
      )}
      <textarea
        ref={box}
        rows={1}
        data-over={over ? "" : undefined}
        value={text}
        placeholder={t("交待一个任务，回车发送…　/ 调用命令与技能，@ 引用文件")}
        role="combobox"
        aria-expanded={menu.open}
        aria-controls="slashmenu"
        aria-autocomplete="list"
        aria-activedescendant={menu.open ? `slash-${menu.active}` : undefined}
        onChange={(e) => type(e.target.value, e.target.selectionStart)}
        // Arrow keys and clicks move the caret without changing the text, and
        // the caret is what decides which token the menu is completing.
        onKeyUp={(e) => setCaret(e.currentTarget.selectionStart)}
        onClick={(e) => setCaret(e.currentTarget.selectionStart)}
        onPaste={(e) => {
          const files = [...e.clipboardData.files];
          if (files.some((f) => f.type.startsWith("image/"))) {
            e.preventDefault();
            grab(files);
          }
        }}
        onDragOver={(e) => {
          e.preventDefault();
          setOver(true);
        }}
        onDragLeave={() => setOver(false)}
        onDrop={(e) => {
          e.preventDefault();
          setOver(false);
          grab([...e.dataTransfer.files]);
        }}
        {...ime.handlers}
        onKeyDown={(e) => {
          // Picking a word from an input method is not typing in this box: its
          // Enter confirms a candidate, and acting on it would send a
          // half-written message or accept a completion nobody asked for.
          if (ime.isIme(e.nativeEvent)) {
            // Esc dismisses the candidate window; letting it through would
            // cancel the running turn as a side effect of closing an IME.
            if (e.key === "Escape") e.stopPropagation();
            return;
          }
          if (menu.open && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
            e.preventDefault();
            menu.move(e.key === "ArrowDown" ? 1 : -1);
            return;
          }
          // Tab completes, always. Enter belongs to the menu only where the
          // line is not yet a message — a half-typed command — or where the
          // user went looking through the list themselves.
          if (menu.open && (e.key === "Tab" || (e.key === "Enter" && menu.ownsEnter)) && !e.shiftKey) {
            e.preventDefault();
            menu.accept();
            return;
          }
          // Esc closes the menu and stops there: reaching the app would cancel
          // the running turn, which is not what dismissing a menu means.
          if (menu.open && e.key === "Escape") {
            e.preventDefault();
            e.stopPropagation();
            menu.dismiss();
            return;
          }
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            send();
          }
          if (e.key === "Tab" && e.shiftKey) {
            e.preventDefault();
            change(port.setPlanMode(!status?.plan));
          }
        }}
      />
      <div className="row" data-busy={switching ? "" : undefined}>
        {/* 拖进来和粘贴都走同一条路，但那两个都得先有一张图在手边。点开系统
            选择器是唯一不需要预备动作的入口。 */}
        <input
          ref={picker}
          type="file"
          accept="image/*"
          multiple
          hidden
          onChange={(e) => {
            grab([...(e.target.files ?? [])]);
            // 同一张图再选一次也要能进来，所以每次用完清空。
            e.target.value = "";
          }}
        />
        <button
          className="mode plain attach"
          title={t("添加图片　也可以直接拖进来或粘贴")}
          aria-label={t("添加图片")}
          onClick={() => picker.current?.click()}
        >
          ＋
        </button>
        <Picker
          className="mode"
          place="bottom"
          current={status?.modelRef}
          items={modelMenu(models)}
          onPick={(ref) => change(port.setModel(ref))}
          label={
            <>
              <span className="dot" aria-hidden="true" />
              <span>{modelLb}</span>
            </>
          }
        />
        <Picker
          className="mode plain"
          place="bottom"
          current={status?.effort || "auto"}
          items={EFFORTS.map((v) => ({ value: v, label: v }))}
          onPick={(v) => change(port.setEffort(v))}
          label={<span>{t("强度")} {status?.effort || "auto"}</span>}
        />
        <button
          className="mode tog"
          aria-pressed={status?.plan ?? false}
          onClick={() => change(port.setPlanMode(!status?.plan))}
        >
          {t("计划")}
        </button>
        <Picker
          className={apv === "yolo" ? "mode plain danger" : "mode plain"}
          place="bottom"
          current={apv}
          items={APPROVALS.map(([v, lb, ds]) => ({ value: v, label: t(lb), desc: t(ds) }))}
          onPick={(v) => change(port.setApprovalMode(v as ApprovalMode))}
          label={<span>{t("批准")} {t(APPROVALS.find(([m]) => m === apv)?.[1] ?? "")}</span>}
        />
        <span className="go">
          <button
            className="btn send"
            data-primary
            onClick={() => (running ? change(port.cancel()) : send())}
          >
            <span className="ic" aria-hidden="true">
              <svg className="i-send" viewBox="0 0 16 16">
                <path d="M2.8 8h9.4M8.4 4.2 12.2 8l-3.8 3.8" />
              </svg>
              <svg className="i-stop" viewBox="0 0 16 16">
                <rect x="4.8" y="4.8" width="6.4" height="6.4" rx="1.3" />
              </svg>
            </span>
            <span>{t(running ? "停下" : "发送")}</span>
          </button>
        </span>
      </div>
    </>
  );
}
