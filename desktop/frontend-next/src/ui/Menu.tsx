import { Fragment, useEffect, useRef, useState, type ReactNode } from "react";

export interface MenuItem {
  value: string;
  label: string;
  desc?: string;
  right?: string;
  plain?: boolean;
  divide?: boolean;
  // A group caption. It labels the rows under it and cannot be chosen, so it
  // is not a button — a menu whose headings take focus is a menu you arrow
  // through twice.
  header?: boolean;
}

interface Props {
  label: ReactNode;
  items: MenuItem[];
  current?: string;
  onPick: (value: string) => void;
  place: "top" | "bottom";
  className?: string;
  title?: string;
}

export function Picker({ label, items, current, onPick, place, className, title }: Props) {
  const [open, setOpen] = useState(false);
  const wrap = useRef<HTMLDivElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const btn = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    menu.current?.querySelector<HTMLElement>(".mi")?.focus();
    const onDown = (e: MouseEvent) => {
      if (!wrap.current?.contains(e.target as Node)) setOpen(false);
    };
    // Esc unwinds one layer at a time: the menu first, the run only once no
    // menu is left, so the capture phase has to stop it reaching the app.
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.stopPropagation();
      setOpen(false);
      btn.current?.focus();
    };
    addEventListener("mousedown", onDown);
    addEventListener("keydown", onKey, true);
    return () => {
      removeEventListener("mousedown", onDown);
      removeEventListener("keydown", onKey, true);
    };
  }, [open]);

  const arrows = (e: React.KeyboardEvent) => {
    const all = [...(menu.current?.querySelectorAll<HTMLElement>(".mi") ?? [])];
    const i = all.indexOf(document.activeElement as HTMLElement);
    const to = e.key === "ArrowDown" ? i + 1 : e.key === "ArrowUp" ? i - 1 : -1;
    if (i < 0 || to < 0 || to >= all.length) return;
    e.preventDefault();
    all[to].focus();
  };

  return (
    <div className="picker" ref={wrap}>
      <button
        ref={btn}
        className={className}
        aria-haspopup="menu"
        aria-expanded={open}
        title={title}
        onClick={() => setOpen((v) => !v)}
      >
        {label}
      </button>
      <div
        ref={menu}
        className={place === "top" ? "menu projmenu" : "menu modemenu"}
        role="menu"
        hidden={!open}
        onKeyDown={arrows}
      >
        {items.map((it) => (
          <Fragment key={it.value}>
            {it.divide && <div className="div" />}
            {it.header ? (
              <div className="mi head">
                <span className="lb">{it.label}</span>
                {it.right && <span className="rt">{it.right}</span>}
              </div>
            ) : (
            <button
              className={it.plain ? "mi plain" : "mi"}
              role="menuitem"
              data-on={it.value === current ? "" : undefined}
              onClick={() => {
                setOpen(false);
                onPick(it.value);
              }}
            >
              <span className="dot" />
              <span className="tx">
                <span className="lb">{it.label}</span>
                {it.desc && <span className="ds">{it.desc}</span>}
              </span>
              {it.right && <span className="rt">{it.right}</span>}
            </button>
            )}
          </Fragment>
        ))}
      </div>
    </div>
  );
}
