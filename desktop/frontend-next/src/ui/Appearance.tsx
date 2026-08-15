import { useCallback, useEffect, useState, type CSSProperties } from "react";
import type { AgentPort, ThemePack } from "../port/port";

// Exported because the nav rail names the current one next to the tab.
export const SCHEMES: [string, string][] = [
  ["auto", "跟随系统"],
  ["light", "浅色"],
  ["dark", "深色"],
];

interface Props {
  port: AgentPort;
  theme: string;
  onTheme: (t: string) => void;
  reloadThemes: () => void;
}

export function Appearance({ port, theme, onTheme, reloadThemes }: Props) {
  const [packs, setPacks] = useState<ThemePack[]>([]);

  const load = useCallback(() => {
    port.themes().then(setPacks).catch(() => setPacks([]));
  }, [port]);
  useEffect(load, [load]);

  // Activating repaints through App's own theme effect, so this only refreshes
  // the list and asks App to re-read which pack is active.
  const pick = useCallback(
    (id: string) => {
      port
        .activateTheme(id)
        .then(() => {
          load();
          reloadThemes();
        })
        .catch(() => {});
    },
    [port, load, reloadThemes],
  );

  const custom = packs.some((p) => p.active);

  return (
    <>
      <section className="grp">
        <div className="grp-hd">
          <h2>明暗</h2>
        </div>
        <p className="hint">跟随系统时，系统切换会立刻反映；手动选过就固定住。</p>
        <div className="grp-items">
          <div className="seg" data-text role="group" aria-label="明暗">
            {SCHEMES.map(([id, name]) => (
              <button key={id} aria-pressed={theme === id} onClick={() => onTheme(id)}>
                {name}
              </button>
            ))}
          </div>
        </div>
      </section>

      <section className="grp">
        <div className="grp-hd">
          <h2>配色</h2>
          <span className="now">{packs.length ? `${packs.length} 个已装` : ""}</span>
        </div>
        <p className="hint">
          装在记忆目录的 themes/ 下，一个目录一个 theme.json。表面、强调色、圆角与字体跟着走；状态色（成功/警告/失败）不跟，那是含义不是装饰。
        </p>
        <div className="grp-items">
          <div className="palettes" role="group" aria-label="配色">
            <Swatch name="默认" on={!custom} onPick={() => pick("")} />
            {packs.map((p) => (
              <Swatch key={p.id} pack={p} theme={theme} name={p.name} on={!!p.active} onPick={() => pick(p.id)} />
            ))}
          </div>
          {/* A pack loads with its good tokens and says which ones it lost. The
              author is the only one who can fix that, and they will not read a
              log — so it is here, next to the thing that looks wrong. */}
          {packs.filter((p) => p.warnings?.length).map((p) => (
            <div className="find" data-lvl="warn" key={p.id}>
              <span className="t">{p.name} 有几项没生效</span>
              {p.warnings?.map((w) => (
                <span className="why" key={w}>
                  {w}
                </span>
              ))}
            </div>
          ))}
          {packs.length === 0 && <p className="note">还没装配色。把一个带 theme.json 的目录放进 themes/ 就会出现在这里。</p>}
        </div>
      </section>
    </>
  );
}

// The card paints itself from the pack's own tokens, so a palette is picked by
// looking at it rather than by reading its name — and a pack that ships no
// picture, or whose picture fails to load, still previews correctly.
function Swatch({
  pack, theme, name, on, onPick,
}: {
  pack?: ThemePack;
  theme?: string;
  name: string;
  on: boolean;
  onPick: () => void;
}) {
  const [shot, setShot] = useState(true);
  const tokens = pack && (pack.tokens[activeScheme(theme)] ?? pack.tokens.light ?? pack.tokens.dark);
  const style = tokens
    ? ({
        "--pal-page": tokens.bg,
        "--pal-surface": tokens.bgSoft ?? tokens.bg,
        "--pal-panel": tokens.panel ?? tokens.bgSoft ?? tokens.bg,
        "--pal-border": tokens.border,
        "--pal-text": tokens.fg,
        "--pal-dim": tokens.fgDim ?? tokens.fg,
        "--pal-accent": tokens.accent,
      } as CSSProperties)
    : undefined;

  return (
    <button className="pal" type="button" title={pack?.description || name} data-on={on ? "" : undefined} aria-pressed={on} onClick={onPick}>
      <span className="pal-art" style={style}>
        <span className="pal-rail" />
        <span className="pal-body">
          <span className="pal-line" />
          <span className="pal-line" data-short />
          <span className="pal-mark" />
        </span>
        {pack?.hasPreview && shot && (
          <img
            className="pal-shot"
            src={`/themes/${encodeURIComponent(pack.id)}/preview`}
            alt=""
            loading="lazy"
            onError={() => setShot(false)}
          />
        )}
      </span>
      <span className="pal-nm">
        <b>{name}</b>
        <em>{pack ? pack.author || "第三方" : "内置"}</em>
      </span>
    </button>
  );
}

// App writes the resolved scheme onto the document, which is the only place
// "auto" has an answer.
function activeScheme(theme?: string): "light" | "dark" {
  if (theme === "light" || theme === "dark") return theme;
  return document.documentElement.dataset.theme === "dark" ? "dark" : "light";
}
