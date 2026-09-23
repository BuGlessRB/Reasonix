// Provider compatibility values are shared by add and edit. Keeping the wire
// vocabulary here prevents the two forms from drifting while letting the UI
// translate the labels independently.
export const THINKING: [string, string][] = [
  ["", "自动 · 按模型和地址推断"],
  ["openai", "OpenAI reasoning_effort"],
  ["anthropic", "Anthropic thinking"],
  ["deepseek", "DeepSeek"],
  ["glm", "GLM enable_thinking"],
  ["kimi-k3", "Kimi K3"],
  ["none", "不发思考参数"],
];

export function headerLines(headers?: Record<string, string>): string {
  return Object.entries(headers ?? {})
    .map(([k, v]) => `${k}: ${v}`)
    .join("\n");
}

export function parseHeaders(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const at = line.indexOf(":");
    if (at <= 0) continue;
    const name = line.slice(0, at).trim();
    const value = line.slice(at + 1).trim();
    if (name && value) out[name] = value;
  }
  return out;
}

// null means "typed but invalid", unlike an empty object which is a deliberate
// request to leave no custom request fields behind.
export function parseExtraBody(text: string): Record<string, unknown> | null {
  if (!text.trim()) return {};
  try {
    const parsed: unknown = JSON.parse(text);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return null;
    return parsed as Record<string, unknown>;
  } catch {
    return null;
  }
}

// Mirrors what the kernel stores: lower-cased, deduplicated, and without auto,
// which every ladder already opens with.
export function parseEffortLevels(text: string): string[] {
  const out: string[] = [];
  for (const raw of text.split(/[\s,，、]+/)) {
    const level = raw.trim().toLowerCase();
    if (level && level !== "auto" && !out.includes(level)) out.push(level);
  }
  return out;
}
