import { SseMcp } from "./sse_mcp";
import type { HookCatalog, HookDryRun, HookEntry } from "./port";

// Commands hung on the agent's own events. They run here with the user's rights,
// so a dry run answers with what the command actually did.
export class SseHook extends SseMcp {
  hooks() {
    return this.get<HookCatalog>("/hooks");
  }
  saveHooks(scope: "user" | "project", hooks: HookEntry[]) {
    return this.post("/hooks", { scope, hooks });
  }

  // The failure message is the answer here — "command not found" is exactly
  // what the user is trying to learn — so it is read out of the body.
  // The failure message is the answer here — "command not found" is exactly
  // what the user is trying to learn — so it is read out of the body.
  async dryRunHook(h: HookEntry): Promise<HookDryRun> {
    const res = await fetch(this.base + "/hooks/dry-run", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ event: h.event, match: h.match, command: h.command, timeout: h.timeout, cwd: h.cwd }),
    });
    const body = (await res.json().catch(() => ({}))) as HookDryRun & { error?: string };
    if (!res.ok) throw new Error(body.error || `/hooks/dry-run: ${res.status}`);
    return body;
  }
}
