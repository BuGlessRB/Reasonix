import type { ThemePack } from "../port/port";

// A pack names things in its own vocabulary; this is where they become ours.
// The mapping lives on this side because only the frontend knows what each
// surface in its layout is called — a pack should not have to learn our
// variable names to be worth installing. The vocabulary itself is the kernel's
// (internal/theme.Tokens), and TestThemeTokenVocabularyMatchesTheFrontend holds
// the two halves together.
const SURFACE: Record<string, string[]> = {
  bg: ["--page"],
  bgSoft: ["--surface"],
  panel: ["--raised"],
  bgElev: ["--overlay"],
  border: ["--border"],
  borderSoft: ["--hair"],
  fg: ["--text"],
  fgDim: ["--muted"],
  fgFaint: ["--faint", "--ghost"],
  accent: ["--accent"],
  accentFg: ["--accent-fg"],
  // Shape and type. --r-pill is absent because a pill is a shape rather than a
  // size: a pack that could set it would round a button into something else.
  radiusXs: ["--r-xs"],
  radiusSm: ["--r-sm"],
  radiusMd: ["--r-md"],
  fontUi: ["--ui"],
  fontMono: ["--mono"],
};

// What a pack may not touch. ok/warn/err/net/deleg encode what is happening —
// "this broke", "this is running", "this went out to a sub-agent" — and a
// theme that could recolour them would let a failure render as success. The
// palette is the theme's; the meanings are the app's.
const RESERVED = ["--ok", "--warn", "--err", "--net", "--deleg", "--add", "--del", "--focus"];

// Variables the picture rides on. They are set together with the colours so a
// pack never lands half-applied — an image over the previous palette.
const IMAGE_VARS = ["--bg-image", "--bg-x", "--bg-y", "--bg-alpha", "--bg-overlay"];

/** apply paints a pack onto the document, or clears back to the stylesheet.
 *  `busy` dims the picture while a turn runs: a photo that is right behind an
 *  idle window is in the way of a transcript being read. */
export function apply(pack: ThemePack | null, scheme: "light" | "dark", busy = false) {
  const root = document.documentElement;
  for (const v of IMAGE_VARS) root.style.removeProperty(v);
  // The flag is what lets the page surface go transparent. It is removed first
  // so a pack without a picture never leaves the previous one's window open.
  delete root.dataset.bg;
  for (const vars of Object.values(SURFACE)) {
    for (const v of vars) root.style.removeProperty(v);
  }
  root.style.removeProperty("--accent-wash");
  if (!pack) return;

  const tokens = pack.tokens[scheme];
  if (!tokens) return;
  for (const [name, value] of Object.entries(tokens)) {
    for (const v of SURFACE[name] ?? []) root.style.setProperty(v, value);
  }
  // The washes are tints of the accent, so a pack that moves the accent has to
  // move them too or the tinted backgrounds keep pointing at the old hue.
  if (tokens.accent) {
    root.style.setProperty("--accent-wash", `color-mix(in srgb, ${tokens.accent} 12%, ${tokens.bg ?? "transparent"})`);
  }

  const bg = pack.background;
  if (!bg?.image) return;
  // The id is the address; the bytes are immutable for it, so the URL needs no
  // cache buster. encodeURIComponent is what keeps a pack id out of the CSS
  // url() grammar.
  root.style.setProperty("--bg-image", `url("/themes/${encodeURIComponent(pack.id)}/background")`);
  root.style.setProperty("--bg-x", `${pct(bg.focusX)}%`);
  root.style.setProperty("--bg-y", `${pct(bg.focusY)}%`);
  root.style.setProperty("--bg-alpha", String(busy ? bg.taskOpacity : bg.homeOpacity));
  root.style.setProperty("--bg-overlay", String(bg.overlayStrength));
  root.dataset.bg = "on";
}

function pct(v: number | undefined): number {
  if (typeof v !== "number" || Number.isNaN(v)) return 50;
  return Math.round(Math.max(0, Math.min(1, v)) * 100);
}

/** reserved is exported for the test that pins the meanings a pack cannot take. */
export const reserved = RESERVED;
