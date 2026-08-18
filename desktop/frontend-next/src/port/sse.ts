import { download } from "./download";
import type { AccountState, AgentPort, Appearance, ContextBreakdown, Completion, DeviceGrant, ProviderCheck, ProviderDraft, ProviderEdit, ProviderEntry, ProviderProbe, UpdateProgress, VersionHub, ApprovalMode, ApprovalVerdict, Checkpoint, RewindPlan, RewindResult, RewindScope, HistoryMessage, ModelEntry, Preset, ProviderSetup, RoleAssignments, SessionEntry, SessionStatus, HookCatalog, HookDryRun, HookEntry, MemoryCatalog, NetworkProbe, NetworkSettings, ShellSettings, PermissionLists, PermissionRules, SandboxSettings, McpCatalog, McpDraft, McpDraftServer, McpInstallResult, McpInstallScope, CapabilityScope, ScopeLayer, PluginExport, PluginInstallRequest, PluginPackage, PluginPlan, SkillCatalog, WorkspaceInfo, ThemePack } from "./port";
import { HttpError, type Attachment, type WorkspaceChanges } from "./port";
import type { WireEvent } from "./wire";

// The running project is the default, so its requests stay the bare path they
// have always been and only a cross-project read carries the folder.
function rootQuery(root?: string): string {
  return root ? "?root=" + encodeURIComponent(root) : "";
}

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

// Wails' own drop API, which is the only subscriber that honours
// --wails-drop-target. Absent in a browser tab, where a page never learns a path.
interface WailsFileDropBus {
  OnFileDrop(cb: (x: number, y: number, paths: string[]) => void, useDropTarget: boolean): void;
}

// Wails publishes bound methods at window.go.<package>.<Struct>.<Method>; the
// shell's package is main and the struct is App. Absent in a browser tab.
interface WailsBind {
  go?: {
    main?: {
      App?: {
        PickWorkspace?: () => Promise<string>;
        OpenExternal?: (url: string) => Promise<void>;
        Versions?: () => Promise<VersionHub>;
        PinVersion?: (version: string) => Promise<void>;
        GoToVersion?: (version: string) => Promise<void>;
        SavePluginExport?: (name: string) => Promise<{ path: string; required: string[] }>;
        SaveText?: (name: string, content: string) => Promise<string>;
      };
    };
  };
}

export class SsePort implements AgentPort {
  // rt names the pane this port speaks for. The shell's bus carries every
  // pane's frames, so a channel per runtime is what keeps two live
  // conversations out of each other's transcript.
  constructor(private readonly base = "", private readonly rt = "") {}

  private readonly dropSubs = new Set<(paths: string[]) => void>();
  private dropWired = false;

  // A refusal carries a code; only when it does not do we fall back to text.
  // Throwing HttpError with the reason attached keeps that choice at the point
  // that renders it, instead of flattening it to a string here.
  private static async fail(path: string, res: Response): Promise<never> {
    const body = (await res.json().catch(() => null)) as
      | { code?: string; error?: string; params?: Record<string, string | number> }
      | null;
    throw new HttpError(res.status, body?.error || `${path}: ${res.status}`, body ?? undefined);
  }

  private async post(path: string, body?: unknown): Promise<void> {
    const res = await fetch(this.base + path, {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!res.ok) await SsePort.fail(path, res);
  }

  // A POST whose answer is the payload, not a status code.
  private async post0<T>(path: string, body?: unknown): Promise<T> {
    const res = await fetch(this.base + path, {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!res.ok) await SsePort.fail(path, res);
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

  complete(line: string, cursor: number) {
    const q = new URLSearchParams({ line, cursor: String(cursor) });
    return this.get<Completion>("/complete?" + q);
  }

  skills(root?: string) {
    return this.get<SkillCatalog>("/skills" + rootQuery(root));
  }

  setSkillEnabled(name: string, enabled: boolean, scope: ScopeLayer = "project", root?: string) {
    return this.post("/skills/enabled", { name, enabled, scope, root });
  }

  clearSkillOverride(name: string, root?: string) {
    return this.post("/skills/enabled", { name, clear: true, scope: "project", root });
  }

  plugins() {
    return this.get<PluginPackage[]>("/plugins");
  }

  planPlugin(req: PluginInstallRequest) {
    return this.post0<PluginPlan>("/plugins/plan", req);
  }

  installPlugin(req: PluginInstallRequest) {
    return this.post0<PluginPlan>("/plugins/install", req);
  }

  async setPluginEnabled(name: string, enabled: boolean) {
    await this.post0<{ reloadError?: string }>("/plugins/enabled", { name, enabled });
  }

  async removePlugin(name: string): Promise<PluginPlan> {
    const res = await fetch(this.base + "/plugins/" + encodeURIComponent(name), {
      method: "DELETE",
      credentials: "same-origin",
    });
    const body = (await res.json().catch(() => ({}))) as PluginPlan & { error?: string };
    if (!res.ok) throw new Error(body.error || `/plugins/${name}: ${res.status}`);
    return body;
  }

  // A webview starts no downloads of its own, so the shell writes the file
  // through its own save dialog when there is one. In a browser tab the archive
  // is an ordinary download, and the header is read first because the body is
  // bytes and has nowhere to say what was stripped out of it.
  async exportPlugin(name: string): Promise<PluginExport> {
    const save = (window as WailsBind).go?.main?.App?.SavePluginExport;
    if (save) {
      const out = await save(name);
      return { required: out.required ?? [], savedTo: out.path || undefined };
    }
    const res = await fetch(this.base + "/plugins/" + encodeURIComponent(name) + "/export", {
      credentials: "same-origin",
    });
    if (!res.ok) throw new Error(`/plugins/${name}/export: ${res.status}`);
    const required = (res.headers.get("X-Reasonix-Required-Env") ?? "").split(",").filter(Boolean);
    const url = URL.createObjectURL(await res.blob());
    const a = document.createElement("a");
    a.href = url;
    a.download = `${name}.zip`;
    a.click();
    URL.revokeObjectURL(url);
    return { required };
  }

  async saveText(name: string, content: string): Promise<string | null> {
    const save = (window as WailsBind).go?.main?.App?.SaveText;
    if (save) return (await save(name, content)) || null;
    download(name, content);
    return null;
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

  permissions() {
    return this.get<PermissionRules>("/permissions");
  }

  savePermissions(lists: PermissionLists) {
    return this.post0<PermissionRules>("/permissions", lists);
  }

  sandbox() {
    return this.get<SandboxSettings>("/sandbox");
  }

  saveSandbox(s: SandboxSettings) {
    return this.post0<SandboxSettings>("/sandbox", s);
  }

  mcp(root?: string) {
    return this.get<McpCatalog>("/mcp" + rootQuery(root));
  }

  capabilityScope() {
    return this.get<CapabilityScope>("/capability-scope");
  }

  async capabilityScopes() {
    const r = await this.get<{ scopes?: CapabilityScope[] }>("/capability-scope?all");
    return r.scopes ?? [];
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

  setMcpEnabled(name: string, enabled: boolean, scope: ScopeLayer = "project", root?: string) {
    return this.post("/mcp/enabled", { name, enabled, scope, root });
  }

  clearMcpOverride(name: string, root?: string) {
    return this.post("/mcp/enabled", { name, clear: true, scope: "project", root });
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

  async installMcp(server: McpDraftServer, scope: McpInstallScope): Promise<McpInstallResult> {
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

  editProvider(edit: ProviderEdit) {
    return this.post("/providers/edit", edit);
  }

  setProviderWebSearch(name: string, on: boolean) {
    return this.post("/providers/websearch", { name, on });
  }

  setProviderThinking(name: string, on: boolean) {
    return this.post("/providers/thinking", { name, on });
  }

  async welcomeSeen(): Promise<boolean> {
    const res = await this.get<{ seen: boolean }>("/welcome");
    return !!res.seen;
  }

  markWelcomed() {
    return this.post("/welcome", {});
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

  // Wails registers its drop listeners once and ignores a second call, so the
  // one subscription is fanned out here. useDropTarget=true is what keeps this
  // to elements that opted in: without it every drop in the window arrives,
  // including an image the composer is about to attach.
  onFileDrop(cb: (paths: string[]) => void): () => void {
    const rt = (window as unknown as { runtime?: WailsFileDropBus }).runtime;
    if (!rt?.OnFileDrop) return () => {};
    if (!this.dropWired) {
      this.dropWired = true;
      rt.OnFileDrop((_x, _y, paths) => this.dropSubs.forEach((f) => f(paths ?? [])), true);
    }
    this.dropSubs.add(cb);
    return () => {
      this.dropSubs.delete(cb);
    };
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

  async openExternal(url: string) {
    const bind = (window as unknown as WailsBind).go?.main?.App?.OpenExternal;
    if (bind) {
      await bind(url);
      return;
    }
    window.open(url, "_blank", "noopener,noreferrer");
  }

  async pickFolder(): Promise<string | null> {
    const bind = (window as unknown as WailsBind).go?.main?.App?.PickWorkspace;
    if (!bind) return null;
    return (await bind()) ?? "";
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

  // JSON, not raw bytes: csrfGuard admits nothing else, and that guard is what
  // stops a cross-site form posting here at all.
  async attach(blob: Blob) {
    const buf = new Uint8Array(await blob.arrayBuffer());
    let bin = "";
    for (let i = 0; i < buf.length; i += 0x8000) bin += String.fromCharCode(...buf.subarray(i, i + 0x8000));
    const res = await fetch(this.base + "/attachments", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ mime: blob.type, data: btoa(bin) }),
    });
    if (!res.ok) throw new HttpError(res.status, `/attachments: ${res.status} ${await res.text()}`);
    return (await res.json()) as Attachment;
  }

  changes() {
    return this.get<WorkspaceChanges>("/changes");
  }

  trajectory() {
    return this.get<WireEvent[]>("/trajectory");
  }

  history() {
    return this.get<HistoryMessage[]>("/history");
  }

  checkpoints() {
    return this.get<Checkpoint[]>("/checkpoints");
  }

  prepareRewind(turn: number, scope: RewindScope) {
    return this.post0<RewindPlan>("/rewind/prepare", { turn, scope });
  }

  commitRewind(planId: string) {
    return this.post0<RewindResult>("/rewind/commit", { planId });
  }

  undoRewind(transactionId: string) {
    return this.post("/rewind/undo", { transactionId });
  }

  /** subscribe delivers the kernel's events, in order, with no holes it did not
   *  announce. Frames that matter are numbered, so a break in the numbers is
   *  the client noticing it missed something rather than a gap it renders as a
   *  quiet turn — and the two transports differ only in how they fetch the
   *  missing frames back, not in what a gap means.
   *
   *  onGap fires when the replay log can no longer close one, which is the
   *  caller's cue to rebuild from the transcript. */
  subscribe(onEvent: (ev: WireEvent) => void, onGap?: () => void) {
    // Frames arriving while a recovery request is in flight wait for it: the
    // whole point is that the reducer sees one ordered stream, and delivering
    // the new frame first would put a result ahead of the dispatch it answers.
    let seen = 0;
    let recovering = false;
    let held: WireEvent[] = [];
    let live = true;

    const deliver = (ev: WireEvent) => {
      if (ev.seq) seen = Math.max(seen, ev.seq);
      onEvent(ev);
    };

    const recover = async (after: number) => {
      recovering = true;
      try {
        const res = await fetch(`${this.base}/events/replay?lastEventId=${after}`, { credentials: "same-origin" });
        if (!res.ok) throw new Error(String(res.status));
        const body = (await res.json()) as { frames?: WireEvent[]; complete?: boolean };
        if (!live) return;
        for (const ev of body.frames ?? []) deliver(ev);
        if (!body.complete) onGap?.();
      } catch {
        // The frames are gone and asking again would not bring them back. The
        // transcript is the one source that can still answer.
        if (live) onGap?.();
      } finally {
        recovering = false;
        const queued = held;
        held = [];
        for (const ev of queued) deliver(ev);
      }
    };

    const accept = (ev: WireEvent) => {
      // The stream describing itself: a watermark states the number the client
      // should have reached, which is the only way to notice a frame lost at
      // the end of a turn. Neither reaches the reducer.
      if (ev.kind === "stream_watermark") {
        if (!recovering && ev.seq && ev.seq > seen) void recover(seen);
        return;
      }
      if (ev.kind === "stream_gap") {
        seen = Math.max(seen, ev.seq ?? 0);
        onGap?.();
        return;
      }
      if (recovering) {
        held.push(ev);
        return;
      }
      // Numbering that goes backwards is a different stream, not a gap: the
      // server restarted and is counting from one again. Inferred rather than
      // carried as a stream id — a resumed client is never sent a lower number
      // than it holds, so nothing else produces this. Without it the client
      // compares against a watermark that will never be reached again and
      // silently stops noticing losses.
      if (ev.seq && seen && ev.seq < seen) {
        seen = ev.seq;
        onGap?.();
        onEvent(ev);
        return;
      }
      if (ev.seq && seen && ev.seq > seen + 1) {
        held.push(ev);
        void recover(seen);
        return;
      }
      deliver(ev);
    };

    const feed = (raw: string) => {
      try {
        accept(JSON.parse(raw) as WireEvent);
      } catch {
        // A malformed frame must not tear down the stream.
      }
    };
    // Wails' asset server buffers a response until its handler returns, so the
    // SSE stream never reaches the page inside the shell. There it pushes the
    // same frames over its own bus; the payload is identical.
    const bus = (window as unknown as { runtime?: WailsBus }).runtime;
    if (bus?.EventsOn) {
      const off = bus.EventsOn(this.rt ? `${WAILS_EVENT}:${this.rt}` : WAILS_EVENT, feed);
      // Subscribing to the bus is not the handshake /events is: ask the shell
      // to replay whatever prompt is already waiting for an answer.
      void fetch(this.base + WAILS_REPLAY, { method: "POST" }).catch(() => {});
      return () => {
        live = false;
        off();
      };
    }

    // EventSource resumes on its own: it reconnects carrying the last id it
    // saw, and the server replays from there. Recovery here is for the frames
    // shed without the connection ever dropping.
    const es = new EventSource(this.base + "/events", { withCredentials: true });
    es.onmessage = (m) => feed(m.data);
    return () => {
      live = false;
      es.close();
    };
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
  context() {
    return this.get<ContextBreakdown>("/context");
  }

  themes() {
    return this.get<ThemePack[]>("/themes");
  }

  appearance() {
    return this.get<Appearance>("/appearance");
  }

  saveAppearance(look: Appearance) {
    return this.post0<Appearance>("/appearance", {
      language: look.language ?? "",
      zoom: look.zoom ?? 0,
      readSize: look.readSize ?? 0,
      fontUi: look.fontUi ?? "",
      fontMono: look.fontMono ?? "",
      opacity: look.wallpaper?.opacity ?? 0,
      dim: look.wallpaper?.dim ?? 0,
      focusX: look.wallpaper?.focusX ?? 0.5,
      focusY: look.wallpaper?.focusY ?? 0.5,
    });
  }

  // JSON, not raw bytes: csrfGuard admits nothing else, which is what stops a
  // cross-site form from posting here at all.
  async uploadWallpaper(blob: Blob) {
    const buf = new Uint8Array(await blob.arrayBuffer());
    let bin = "";
    for (let i = 0; i < buf.length; i += 0x8000) bin += String.fromCharCode(...buf.subarray(i, i + 0x8000));
    const res = await fetch(this.base + "/appearance/wallpaper", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ mime: blob.type, data: btoa(bin) }),
    });
    const body = (await res.json().catch(() => ({}))) as Appearance & { error?: string };
    if (!res.ok) throw new Error(body.error || `/appearance/wallpaper: ${res.status}`);
    return body;
  }

  async clearWallpaper() {
    const res = await fetch(this.base + "/appearance/wallpaper", {
      method: "DELETE",
      credentials: "same-origin",
    });
    if (!res.ok) throw new Error(`/appearance/wallpaper: ${res.status}`);
  }
  async surfaceSlots() {
    const r = await this.get<{ slots?: Record<string, string> }>("/surfaces");
    return r.slots ?? {};
  }

  assignSurface(surface: string, slot: string) {
    return this.post("/surfaces", { surface, slot });
  }

  activateTheme(id: string) {
    return this.post("/themes", { id });
  }
  // The extension's own message is the result, so this reads the body rather
  // than a status code. A refused action answers 422 with its reason.
  async invokeExtensionAction(name: string) {
    const out = await this.post0<{ message?: string }>("/extensions/action", { name });
    return out.message ?? "";
  }
  submitExtensionForm(pluginId: string, surfaceId: string, values: Record<string, unknown>) {
    return this.post("/extensions/submit", { pluginId, surfaceId, values });
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
    return this.post("/goal", { goal: text });
  }
}
