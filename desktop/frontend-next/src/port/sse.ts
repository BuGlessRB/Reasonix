import type { AgentPort, ApprovalMode, ApprovalVerdict, HistoryMessage, ModelEntry, Preset, ProviderSetup, SessionEntry, SessionStatus, McpEntry, SlashEntry, WorkspaceInfo } from "./port";
import type { WireEvent } from "./wire";

// Must match wailsEventName / replayPath in desktop/next.
const WAILS_EVENT = "rx:event";
const WAILS_REPLAY = "/rx-replay";

interface WailsBus {
  EventsOn(name: string, cb: (data: string) => void): () => void;
}

// Wails publishes bound methods at window.go.<package>.<Struct>.<Method>; the
// shell's package is main and the struct is App. Absent in a browser tab.
interface WailsBind {
  go?: { main?: { App?: { PickWorkspace?: () => Promise<string> } } };
}

export class SsePort implements AgentPort {
  constructor(private readonly base = "") {}

  private async post(path: string, body?: unknown): Promise<void> {
    const res = await fetch(this.base + path, {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`${path}: ${res.status} ${await res.text()}`);
  }

  private async get<T>(path: string): Promise<T> {
    const res = await fetch(this.base + path, { credentials: "same-origin" });
    if (!res.ok) throw new Error(`${path}: ${res.status}`);
    return (await res.json()) as T;
  }

  status() {
    return this.get<SessionStatus>("/status");
  }

  async providerSetup(): Promise<ProviderSetup | null> {
    const res = await fetch(this.base + "/provider-setup", { credentials: "same-origin" });
    if (res.status === 404) return null;
    if (!res.ok) throw new Error(`/provider-setup: ${res.status}`);
    return (await res.json()) as ProviderSetup;
  }

  saveProviderKey(apiKey: string) {
    return this.post("/provider-setup", { apiKey });
  }

  async models() {
    const r = await this.get<{ models?: ModelEntry[] }>("/models");
    return r.models ?? [];
  }

  slash() {
    return this.get<SlashEntry[]>("/slash");
  }

  mcp() {
    return this.get<McpEntry[]>("/mcp");
  }

  workspaces() {
    return this.get<WorkspaceInfo>("/workspaces");
  }

  setWorkspace(path: string) {
    return this.post("/workspace", { path });
  }

  isolateWorkspace() {
    return this.post("/workspace", { isolate: true });
  }

  async pickFolder() {
    const bind = (window as unknown as WailsBind).go?.main?.App?.PickWorkspace;
    return bind ? ((await bind()) ?? "") : "";
  }

  sessions() {
    return this.get<SessionEntry[]>("/sessions");
  }

  // /resume swaps the session file on the same controller — serve drives one
  // session at a time by design ("multiple browser tabs share it").
  resume(path: string) {
    return this.post("/resume", { path });
  }

  newSession() {
    return this.post("/new");
  }

  deleteSession(name: string) {
    return this.post("/delete-session", { name });
  }

  trajectory() {
    return this.get<WireEvent[]>("/trajectory");
  }

  history() {
    return this.get<HistoryMessage[]>("/history");
  }

  subscribe(onEvent: (ev: WireEvent) => void) {
    const feed = (raw: string) => {
      try {
        onEvent(JSON.parse(raw) as WireEvent);
      } catch {
        // A malformed frame must not tear down the stream.
      }
    };
    // Wails' asset server buffers a response until its handler returns, so the
    // SSE stream never reaches the page inside the shell. There it pushes the
    // same frames over its own bus; the payload is identical.
    const bus = (window as unknown as { runtime?: WailsBus }).runtime;
    if (bus?.EventsOn) {
      const off = bus.EventsOn(WAILS_EVENT, feed);
      // Subscribing to the bus is not the handshake /events is: ask the shell
      // to replay whatever prompt is already waiting for an answer.
      void fetch(this.base + WAILS_REPLAY, { method: "POST" }).catch(() => {});
      return off;
    }

    const es = new EventSource(this.base + "/events", { withCredentials: true });
    es.onmessage = (m) => feed(m.data);
    return () => es.close();
  }

  submit(text: string) {
    return this.post("/submit", { input: text });
  }
  steer(text: string) {
    return this.post("/inbox/items", { input: text, intent: "steer" });
  }
  cancel() {
    return this.post("/cancel");
  }
  // Approve(id, allow, session, persist) — "always" is a session grant, not a
  // persisted config change.
  approve(id: string, verdict: ApprovalVerdict) {
    return this.post("/approve", {
      id,
      allow: verdict !== "deny",
      session: verdict === "always",
      persist: false,
    });
  }
  answer(id: string, answers: { questionId: string; selected: string[] }[]) {
    return this.post("/answer", {
      id,
      answers: answers.map((a) => ({ QuestionID: a.questionId, Selected: a.selected })),
    });
  }
  setPlanMode(on: boolean) {
    return this.post("/plan", { on });
  }
  setApprovalMode(mode: ApprovalMode) {
    return this.post("/tool-approval-mode", { mode });
  }
  setPreset(preset: Preset) {
    return this.post("/preset", { preset });
  }
  setModel(ref: string) {
    return this.post("/model", { ref });
  }
  setEffort(effort: string) {
    return this.post("/effort", { effort });
  }
  setGoal(text: string) {
    return this.post("/goal", { text });
  }
}
