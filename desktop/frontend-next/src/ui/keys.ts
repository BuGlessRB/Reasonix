// Which modifier this platform spells its shortcuts with.
//
// Read from the agent string, not from the shell's `data-platform`: that one is
// written after a promise resolves, and a label rendered before it would keep
// saying ⌘ on Windows for the rest of the session. The browser build has no
// shell to ask at all.
const mac = /mac|iphone|ipad|ipod/i.test(
  (navigator as { userAgentData?: { platform?: string } }).userAgentData?.platform ?? navigator.userAgent,
);

// chord renders one shortcut the way its platform writes it. macOS sets the
// glyph tight against the key; every other platform spells the word and needs
// the space to stay readable.
export function chord(key: string): string {
  return mac ? `⌘${key}` : `Ctrl ${key}`;
}

// asksDelete reads the key that deletes whatever holds focus: Delete everywhere,
// and ⌘⌫ on macOS, whose main key sends Backspace.
export function asksDelete(ev: { key: string; metaKey: boolean; ctrlKey: boolean; altKey: boolean; shiftKey: boolean }): boolean {
  if (ev.ctrlKey || ev.altKey || ev.shiftKey) return false;
  if (ev.key === "Delete") return !ev.metaKey;
  return mac && ev.metaKey && ev.key === "Backspace";
}
