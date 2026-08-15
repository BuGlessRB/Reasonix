import type { AccountState, AgentPort, DeviceGrant, ProviderCheck, ProviderDraft, ProviderEntry, ProviderProbe, UpdateProgress, VersionHub, ApprovalMode, ApprovalVerdict, HistoryMessage, ModelEntry, Preset, ProviderSetup, RoleAssignments, SessionEntry, SessionStatus, HookCatalog, HookDryRun, HookEntry, MemoryCatalog, NetworkProbe, NetworkSettings, McpDraft, McpDraftServer, McpEntry, McpInstallResult, SkillCatalog, SlashEntry, WorkspaceInfo } from "./port";
import type { WireEvent } from "./wire";

// Must match wailsEventName / replayPath in desktop/next.
const WAILS_EVENT = "rx:event";
// Install progress rides its own channel: it is the shell reporting on itself,
// not something the kernel emitted into the conversation.
const WAILS_UPDATE_EVENT = "rx:update";
const WAILS_REPLAY = "/rx-replay";

interface WailsBus {
  EventsOn(name: string, cb: (data: string) => void): () => void;
}

// The same bus, seen by a caller whose payload is an object rather than the
// event stream's JSON string.
interface WailsUpdateBus {
  EventsOn(name: string, cb: (data: UpdateProgress) => void): () => void;
}

// Wails publishes bound methods at window.go.<package>.<Struct>.<Method>; the
// shell's package is main and the struct is App. Absent in a browser tab.
interface WailsBind {
  go?: {
    main?: {
      App?: {
        PickWorkspace?: () => Promise<string>;
        Versions?: () => Promise<VersionHub>;
        PinVersion?: (version: string) => Promise<void>;
        GoToVersion?: (version: string) => Promise<void>;
      };
    };
  };
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

  // A POST whose answer is the payload, not a status code.
  private async post0<T>(path: string): Promise<T> {
    const res = await fetch(this.base + path, {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
    });
    if (!res.ok) throw new Error(`${path}: ${res.status}`);
    return (await res.json()) as T;
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

  skills() {
    return this.get<SkillCatalog>("/skills");
  }

  setSkillEnabled(name: string, enabled: boolean) {
    return this.post("/skills/enabled", { name, enabled });
  }

  hooks() {
    return this.get<HookCatalog>("/hooks");
  }

  saveHooks(scope: "user" | "project", hooks: HookEntry[]) {
    return this.post("/hooks", { scope, hooks });
  }

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

  memories() {
    return this.get<MemoryCatalog>("/memory");
  }

  forgetMemory(name: string) {
    return this.post("/memory/forget", { name });
  }

  network() {
    return this.get<NetworkSettings>("/network");
  }

  async saveNetwork(settings: NetworkSettings, password: string, clearPassword: boolean) {
    const res = await fetch(this.base + "/network", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ ...settings, password, clearPassword }),
    });
    const body = (await res.json().catch(() => ({}))) as NetworkSettings & { error?: string };
    if (!res.ok) throw new Error(body.error || `/network: ${res.status}`);
    return body;
  }

  async diagnoseNetwork() {
    const r = await this.post0<{ probes?: NetworkProbe[] }>("/network/diagnose");
    return r.probes ?? [];
  }

  mcp() {
    return this.get<McpEntry[]>("/mcp");
  }

  // 502 carries the diagnosis in the body — the reason the retry failed is the
  // whole point of the button, so it is read rather than thrown away.
  async reconnectMcp(name: string) {
    const res = await fetch(this.base + "/mcp/reconnect", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ name }),
    });
    const body = (await res.json().catch(() => ({}))) as { state?: string; tools?: number; error?: string };
    if (!res.ok && !body.error) throw new Error(`/mcp/reconnect: ${res.status}`);
    return { state: body.state ?? (res.ok ? "ready" : "failed"), tools: body.tools, error: body.error };
  }

  setMcpEnabled(name: string, enabled: boolean) {
    return this.post("/mcp/enabled", { name, enabled });
  }

  // A parse failure is the normal case while typing, and its message is the
  // whole feedback — so it is thrown as text, not as a status code.
  async parseMcp(input: string): Promise<McpDraft> {
    const res = await fetch(this.base + "/mcp/parse", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ input }),
    });
    const body = (await res.json().catch(() => ({}))) as McpDraft & { error?: string };
    if (!res.ok) throw new Error(body.error || `/mcp/parse: ${res.status}`);
    return { servers: body.servers ?? [], risks: body.risks ?? [] };
  }

  async installMcp(server: McpDraftServer, scope: "user" | "project"): Promise<McpInstallResult> {
    const res = await fetch(this.base + "/mcp/install", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ server, scope }),
    });
    const body = (await res.json().catch(() => ({}))) as McpInstallResult & { error?: string };
    if (!res.ok) throw new Error(body.error || `/mcp/install: ${res.status}`);
    return body;
  }

  async removeMcp(name: string) {
    const res = await fetch(this.base + "/mcp/remove", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ name }),
    });
    const body = (await res.json().catch(() => ({}))) as {
      disconnected?: boolean;
      stillConfigured?: boolean;
      error?: string;
    };
    if (!res.ok) throw new Error(body.error || `/mcp/remove: ${res.status}`);
    return { disconnected: !!body.disconnected, stillConfigured: !!body.stillConfigured };
  }

  providers() {
    return this.get<ProviderEntry[]>("/providers");
  }

  // The failure message is the answer here — a 401, a wrong path and "no chat
  // models" send the user to three different fixes — so it is read out of the
  // body rather than thrown away as a status code.
  async probeProvider(baseUrl: string, apiKey: string): Promise<ProviderProbe> {
    const res = await fetch(this.base + "/providers/probe", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ baseUrl, apiKey }),
    });
    const text = await res.text();
    if (!res.ok) throw new Error(text.trim() || `/providers/probe: ${res.status}`);
    return JSON.parse(text) as ProviderProbe;
  }

  saveProvider(draft: ProviderDraft) {
    return this.post("/providers", draft);
  }

  roles() {
    return this.get<RoleAssignments>("/roles");
  }

  setRole(role: string, ref: string) {
    return this.post("/roles", { role, ref });
  }

  // Like the add-a-source probe, the interesting answer is in the body: a
  // refused key and a moved endpoint are different fixes.
  async checkProvider(name: string): Promise<ProviderCheck> {
    const res = await fetch(this.base + "/providers/check", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ name }),
    });
    const text = await res.text();
    if (!res.ok) throw new Error(text.trim() || `/providers/check: ${res.status}`);
    return JSON.parse(text) as ProviderCheck;
  }

  removeProvider(name: string) {
    return this.post("/providers/remove", { name });
  }

  async versions(): Promise<VersionHub> {
    const bind = (window as unknown as WailsBind).go?.main?.App?.Versions;
    if (!bind) return { current: "", pinned: "", stalePin: false, latest: "", newer: false, versions: [] };
    return (await bind()) as VersionHub;
  }

  async pinVersion(version: string): Promise<void> {
    const bind = (window as unknown as WailsBind).go?.main?.App?.PinVersion;
    if (bind) await bind(version);
  }

  async goToVersion(version: string): Promise<void> {
    const bind = (window as unknown as WailsBind).go?.main?.App?.GoToVersion;
    if (!bind) throw new Error("浏览器里没有可以更新的安装，请在应用内操作");
    await bind(version);
  }

  onUpdateProgress(cb: (p: UpdateProgress) => void): () => void {
    const bus = (window as unknown as { runtime?: WailsUpdateBus }).runtime;
    if (!bus?.EventsOn) return () => {};
    return bus.EventsOn(WAILS_UPDATE_EVENT, cb);
  }

  account() {
    return this.get<AccountState>("/account");
  }

  accountLogin() {
    return this.post0<DeviceGrant>("/account/login");
  }

  async accountPoll(deviceCode: string) {
    const res = await fetch(this.base + "/account/poll", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ deviceCode }),
    });
    if (!res.ok) throw new Error(await res.text());
    return (await res.json()) as { status: "pending" | "complete"; slowDown?: boolean };
  }

  accountLogout() {
    return this.post("/account/logout");
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
