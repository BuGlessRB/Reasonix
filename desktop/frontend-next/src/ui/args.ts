const ARG_KEYS = ["command", "path", "file_path", "pattern", "query", "url", "description", "prompt", "step_id"];

export function shortArgs(raw: string) {
  if (!raw) return "";
  try {
    const v = JSON.parse(raw);
    for (const k of ARG_KEYS) {
      if (typeof v[k] === "string" && v[k]) return v[k].replace(/\s+/g, " ").slice(0, 96);
    }
    return "";
  } catch {
    return raw.replace(/\s+/g, " ").slice(0, 96);
  }
}

export function argOf(raw: string | undefined, ...keys: string[]): string {
  if (!raw) return "";
  try {
    const v = JSON.parse(raw);
    for (const k of keys) if (typeof v[k] === "string" && v[k]) return v[k];
  } catch {
    return "";
  }
  return "";
}

export function splitPath(p: string): [string, string] {
  const at = Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\"));
  return at < 0 ? ["", p] : [p.slice(0, at + 1), p.slice(at + 1)];
}
