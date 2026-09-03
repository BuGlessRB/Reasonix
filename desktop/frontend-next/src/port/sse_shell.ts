import { SseExtensions } from "./sse_ext";
import type { ContextBreakdown, ShellSettings } from "./port";

// What the turn runs inside: the shell it shells out to, and how much of the
// window the context has left.
export class SseShell extends SseExtensions {
  context() {
    return this.get<ContextBreakdown>("/context");
  }
  // post0 so the refusal keeps its code: "not while a turn is running" and "this
  // server does not let you edit sources" send the reader to different places.
  setContextWindow(window: number) {
    return this.post0<ContextBreakdown>("/context/window", { window });
  }
  shell() {
    return this.get<ShellSettings>("/shell");
  }
  async saveShell(prefer: string, path: string) {
    const res = await fetch(this.base + "/shell", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ prefer, path }),
    });
    const body = (await res.json().catch(() => ({}))) as ShellSettings & { error?: string };
    if (!res.ok) throw new Error(body.error || `/shell: ${res.status}`);
    return body;
  }
}
