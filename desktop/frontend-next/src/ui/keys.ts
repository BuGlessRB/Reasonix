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
