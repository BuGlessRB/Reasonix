// Fixtures for the add-a-server flow: what a draft looks like once accepted,
// and what the kernel would flag about it before it is.
import type { McpDraftServer, McpEntry, McpRisk } from "./port";

export function toEntry(s: McpDraftServer): McpEntry {
  return {
    name: s.name,
    state: "idle",
    enabled: true,
    transport: s.transport,
    source: "user config",
    tools: 0,
  };
}

const SECRETY = /auth|token|secret|credential|api[-_]?key|cookie/i;

export function risksOf(servers: McpDraftServer[]): McpRisk[] {
  const out: McpRisk[] = [];
  for (const s of servers) {
    if (s.command) {
      out.push({ server: s.name, kind: "shell", field: "command", detail: [s.command, ...(s.args ?? [])].join(" ") });
    }
    if (s.url) out.push({ server: s.name, kind: "unknown-host", field: "url", detail: s.url });
    for (const [k, v] of Object.entries(s.env ?? {})) {
      if (SECRETY.test(k) && !v.startsWith("$")) {
        out.push({ server: s.name, kind: "secret", field: "env." + k,
          detail: `明文写进配置文件；改成 \${${k.toUpperCase()}} 可以只留在环境变量里` });
      }
    }
  }
  return out;
}
