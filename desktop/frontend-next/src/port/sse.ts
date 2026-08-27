import type { AccountState, AgentPort, Appearance, Completion, DeviceGrant, ProviderProbe, UpdateProgress, VersionHub, ApprovalMode, ApprovalVerdict, Checkpoint, RewindPlan, RewindResult, RewindScope, HistoryMessage, ModelEntry, Preset, ProviderSetup, RoleAssignments, SessionEntry, SessionStatus, HookDryRun, HookEntry, MemoryCatalog, MemoryEdit, MemoryEntry, UsageReport, McpDraft, PluginExport, Queue, Queued, TrayPrefs, WorkspaceInfo } from "./port";
import { HttpError, type Attachment, type ChangeDiff, type DroppedRef, type WorkspaceChanges } from "./port";
import { SseTheme } from "./sse_theme";
import type { WailsBind } from "./wails";
import type { StoragePlan, StorageState } from "./storage";
import type { WireEvent } from "./wire";
import type { RemoteAsk } from "./remote";

// The running project is the default, so its requests stay the bare path they
// have always been and only a cross-project read carries the folder.
// Must match wailsEventName / replayPath in desktop/next.
const WAILS_EVENT = "rx:event";
// Install progress rides its own channel: it is the shell reporting on itself,
// not something the kernel emitted into the conversation.
const WAILS_UPDATE_EVENT = "rx:update";
// Must match wailsRemoteAsk in desktop/next.
const WAILS_REMOTE_ASK = "reasonix:remote-ask";
const WAILS_REPLAY = "/rx-replay";

interface WailsBus {
  EventsOn(name: string, cb: (data: string) => void): () => void;
}

// The same bus, seen by a caller whose payload is an object rather than the
// event stream's JSON string.
interface WailsUpdateBus {
  EventsOn(name: string, cb: (data: UpdateProgress) => void): () => void;
}

interface WailsAskBus {
  EventsOn(name: string, cb: (data: RemoteAsk) => void): () => void;
}

// Wails' own drop API: the only channel that reports where a dropped file
// lives. Absent in a browser tab, where a page never learns a path.
interface WailsFileDropBus {
  OnFileDrop(cb: (x: number, y: number, paths: string[]) => void, useDropTarget: boolean): void;
}

// Wails publishes bound methods at window.go.<package>.<Struct>.<Method>; the
// shell's package is main and the struct is App. Absent in a browser tab.

export class SsePort extends SseTheme implements AgentPort {
  private readonly dropSubs = new Set<(paths: string[]) => void>();
  private dropWired = false;

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

  usage(days: number, source?: string) {
    const q = new URLSearchParams({ days: String(days) });
    if (source && source !== "all") q.set("source", source);
    return this.get<UsageReport>("/usage?" + q);
  }
  memories() {
    return this.get<MemoryCatalog>("/memory");
  }

  prepareFileRevert(path: string) {
    return this.post0<RewindPlan>("/rewind/file/prepare", { path });
  }
  commitFileRevert(planId: string, resolution?: string) {
    return this.post0<RewindResult>("/rewind/file/commit", { planId, resolution });
  }
  saveMemory(edit: MemoryEdit) {
    return this.post("/memory/save", edit);
  }
  forgetMemory(name: string) {
    return this.post("/memory/forget", { name });
  }
  async memoryRevisions(name: string) {
    const r = await this.get<{ revisions: MemoryEntry[] }>("/memory/revisions?name=" + encodeURIComponent(name));
    return r.revisions ?? [];
  }
  restoreMemory(name: string, revision: number) {
    return this.post("/memory/restore", { name, revision });
  }

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

  storage() {
    return this.get<StorageState>("/storage");
  }

  // Both answer with a plan: a refused move is reported in the body rather
  // than as a failed request, because its refusals are what the panel shows.
  planStorageMove(root: string, dir: string) {
    return this.post0<StoragePlan>("/storage/plan", { root, dir });
  }

  moveStorage(root: string, dir: string) {
    return this.post0<StoragePlan>("/storage/move", { root, dir });
  }

  async versions(): Promise<VersionHub> {
    const bind = (window as unknown as WailsBind).go?.main?.App?.Versions;
    if (!bind) return { current: "", pinned: "", stalePin: false, latest: "", newer: false, versions: [] };
    return (await bind()) as VersionHub;
  }

  async trayPrefs(): Promise<TrayPrefs | null> {
    const bind = (window as unknown as WailsBind).go?.main?.App?.TrayPrefs;
    return bind ? await bind() : null;
  }

  async setTrayPrefs(icon: boolean, closeToTray: boolean): Promise<TrayPrefs | null> {
    const bind = (window as unknown as WailsBind).go?.main?.App?.SetTrayPrefs;
    return bind ? await bind(icon, closeToTray) : null;
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
  // one subscription is fanned out here. useDropTarget=false because the filter
  // it offers is the wrong one: it hit-tests the drop coordinates against the
  // CSS opt-in, and those coordinates are native pixels, which stop agreeing
  // with CSS pixels the moment the interface is zoomed. The page routes the
  // drop against the DOM instead, where the element under the pointer is a
  // fact rather than an arithmetic result.
  // A connect is blocked while one of these is on screen, so there is no
  // polling to fall back on: no bus means no window, and no window means the
  // kernel refused the question rather than asking it.
  onRemoteAsk(cb: (ask: RemoteAsk) => void): () => void {
    const bus = (window as unknown as { runtime?: WailsAskBus }).runtime;
    if (!bus?.EventsOn) return () => {};
    return bus.EventsOn(WAILS_REMOTE_ASK, cb);
  }

  answerRemote(id: string, ok: boolean, text: string) {
    void (window as unknown as WailsBind).go?.main?.App?.AnswerRemote?.(id, ok, text);
  }

  onDroppedPaths(cb: (paths: string[]) => void): () => void {
    const rt = (window as unknown as { runtime?: WailsFileDropBus }).runtime;
    if (!rt?.OnFileDrop) return () => {};
    if (!this.dropWired) {
      this.dropWired = true;
      rt.OnFileDrop((_x, _y, paths) => this.dropSubs.forEach((f) => f(paths ?? [])), false);
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
  async attach(blob: Blob, name?: string) {
    const buf = new Uint8Array(await blob.arrayBuffer());
    let bin = "";
    for (let i = 0; i < buf.length; i += 0x8000) bin += String.fromCharCode(...buf.subarray(i, i + 0x8000));
    const res = await fetch(this.base + "/attachments", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ mime: blob.type, name: name ?? "", data: btoa(bin) }),
    });
    if (!res.ok) throw new HttpError(res.status, `/attachments: ${res.status} ${await res.text()}`);
    return (await res.json()) as Attachment;
  }

  dropRefs(paths: string[]) {
    return this.post0<DroppedRef[]>("/drop", { paths });
  }

  changes() {
    return this.get<WorkspaceChanges>("/changes");
  }

  changeDiff(path: string) {
    return this.get<ChangeDiff>(`/changes/diff?path=${encodeURIComponent(path)}`);
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
    // null until the stream states a position: 0 cannot tell a subscriber that
    // just attached from one that has seen the stream start.
    let seen: number | null = null;
    let recovering = false;
    let held: WireEvent[] = [];
    let live = true;

    const deliver = (ev: WireEvent) => {
      if (ev.seq) seen = Math.max(seen ?? 0, ev.seq);
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
        // The first one states where this subscriber attached: a live-only
        // subscription replays nothing before it, so nothing before it was lost.
        if (seen === null) {
          seen = ev.seq ?? 0;
          return;
        }
        if (!recovering && ev.seq && ev.seq > seen) void recover(seen);
        return;
      }
      if (ev.kind === "stream_gap") {
        seen = Math.max(seen ?? 0, ev.seq ?? 0);
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
    return this.post0<Queued>("/inbox/items", { input: text, intent: "steer" });
  }
  queueFollowup(text: string) {
    return this.post0<Queued>("/inbox/items", { input: text, intent: "followup" });
  }
  async cancelQueued(itemId: string) {
    await this.del("/inbox/items/" + encodeURIComponent(itemId));
  }
  async queue() {
    const q = await this.get<Queue>("/inbox");
    // An empty queue arrives as null, not []: that is what a Go nil slice
    // encodes to. Normalising here is what keeps every reader from having to
    // know it — and one that forgets only fails when the queue runs dry.
    return { ...q, items: q.items ?? [] };
  }
  async readQueued(itemId: string) {
    const r = await this.get<{ envelope?: { displayText?: string } }>("/inbox/items/" + encodeURIComponent(itemId));
    return r.envelope?.displayText ?? "";
  }
  editQueued(itemId: string, text: string) {
    return this.patch("/inbox/items/" + encodeURIComponent(itemId), { input: text });
  }
  moveQueued(itemId: string, toIndex: number) {
    return this.post("/inbox/move", { id: itemId, toIndex });
  }
  retryQueued(itemId: string) {
    return this.post("/inbox/items/" + encodeURIComponent(itemId) + "/retry");
  }
  refreshQueued(itemId: string) {
    return this.post("/inbox/items/" + encodeURIComponent(itemId) + "/refresh");
  }
  setQueuePaused(paused: boolean) {
    return this.post(paused ? "/inbox/pause" : "/inbox/resume");
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
