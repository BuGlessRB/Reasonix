import type { SlashEntry } from "../port/port";

const SCOPE: Record<string, string> = {
  project: "项目",
  custom: "自定义",
  global: "我的",
  builtin: "内置",
};

// The name is only still being typed while the first word is open: once there
// is whitespace the rest is the argument, and the menu has no say in it.
export function slashQuery(text: string): string | null {
  if (!text.startsWith("/")) return null;
  const head = text.slice(1);
  return /\s/.test(head) ? null : head;
}

// Prefix matches first, substring after; the kernel's own order survives inside
// each bucket, so a custom command still wins over a skill of the same name.
export function slashMatches(entries: SlashEntry[], query: string): SlashEntry[] {
  const q = query.toLowerCase();
  if (!q) return entries;
  const starts: SlashEntry[] = [];
  const inside: SlashEntry[] = [];
  for (const e of entries) {
    const name = e.name.toLowerCase();
    if (name.startsWith(q)) starts.push(e);
    else if (name.includes(q)) inside.push(e);
  }
  return [...starts, ...inside];
}

const provenance = (e: SlashEntry) =>
  e.plugin || (e.kind === "command" ? "命令" : SCOPE[e.scope ?? ""] || e.scope || "技能");

interface Props {
  items: SlashEntry[];
  active: number;
  onPick: (e: SlashEntry) => void;
  onHover: (i: number) => void;
}

export function SlashMenu({ items, active, onPick, onHover }: Props) {
  return (
    <div className="menu slashmenu" id="slashmenu" role="listbox" aria-label="斜杠命令">
      {items.map((it, i) => (
        <button
          key={it.kind + ":" + it.name}
          id={`slash-${i}`}
          className="mi"
          role="option"
          aria-selected={i === active}
          data-on={i === active ? "" : undefined}
          tabIndex={-1}
          onMouseMove={() => onHover(i)}
          // mousedown, not click: the textarea must not lose focus first.
          onMouseDown={(e) => {
            e.preventDefault();
            onPick(it);
          }}
        >
          <span className="dot" />
          <span className="tx">
            <span className="lb">
              /{it.name}
              {it.argHint && <i className="ah">{it.argHint}</i>}
              {it.subagent && <i className="sa">子代理</i>}
            </span>
            {it.description && <span className="ds">{it.description}</span>}
          </span>
          <span className="rt">{provenance(it)}</span>
        </button>
      ))}
    </div>
  );
}
