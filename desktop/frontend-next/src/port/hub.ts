import { HttpError, type AgentPort } from "./port";
import type { RemoteAsk, RemoteHost, RemoteHostEdit, RemoteListing, RemoteProbe } from "./remote";
import { SsePort } from "./sse";

// RuntimeView is one open pane. Base is the prefix every request of that pane
// carries, which is also what tells the shell's bus which channel to listen on.
export interface RuntimeView {
  id: string;
  base: string;
  root: string;
  name: string;
  sessionPath?: string;
  // Set only on a pane whose kernel runs on another machine, naming it. Absence
  // is what marks a pane as this machine's own, so the common case stays plain.
  host?: string;
}

export interface TreeSession {
  path: string;
  name: string;
  title?: string;
  turns?: number;
  // Set when a pane already drives this transcript: the sidebar focuses that
  // pane rather than opening a second writer for one file.
  runtimeId?: string;
  // Conflict-recovery copies of this same conversation. A save that keeps
  // conflicting writes one per turn, all under one title.
  copies?: TreeSession[];
}

export interface TreeWorkspace {
  root: string;
  name: string;
  // A delivery worktree carries the project's own folder name, so the sidebar
  // needs this to tell the copy from the original.
  isolated?: boolean;
  missing?: boolean;
  open?: boolean;
  // True when the user chose this folder. A window always has a root — the one
  // it happened to launch in — and only this tells that apart from a project.
  remembered?: boolean;
  sessions: TreeSession[];
}

// HubPort is the window's view of the kernel: which panes it drives and which
// folders it can open. One AgentPort hangs off each pane.
export interface HubPort {
  runtimes(): Promise<RuntimeView[]>;
  // The pane ceiling this kernel reported, so a control greys out at the right
  // count rather than at a number the client guessed.
  maxPanes(): number;
  open(req: { root?: string; sessionPath?: string }): Promise<RuntimeView>;
  close(id: string): Promise<void>;
  tree(): Promise<TreeWorkspace[]>;
  addWorkspace(path: string): Promise<TreeWorkspace>;
  removeWorkspace(path: string): Promise<void>;
  removeSession(path: string): Promise<void>;
  renameSession(path: string, title: string): Promise<void>;
  // The host book with each link's state, or null where this kernel refuses
  // remote panes outright — a page served to a browser, rather than the window.
  // Null is what lets the sidebar leave the whole section out instead of
  // drawing a header over a feature that cannot work here.
  remoteHosts(): Promise<RemoteHost[] | null>;
  // Opens a pane on another machine. Slow the first time — the kernel may be
  // installing itself over there — so the caller polls remoteHosts for the step.
  openRemote(req: { host: string; workspace?: string; sessionPath?: string }): Promise<RuntimeView>;
  // The far machine's own workspaces, or null while nothing is open on it:
  // there is no kernel over there to ask until a pane puts one there.
  remoteTree(host: string): Promise<TreeWorkspace[] | null>;
  // Folders on the far machine, read over the link alone — no kernel has to be
  // installed over there to answer it, which is why picking one works on a
  // machine nothing is open on. An empty path is that machine's login home.
  remoteDirs(host: string, path?: string): Promise<RemoteListing>;
  /** What that machine can do, without changing anything on it. A cold connect
   *  stops at the first missing piece; this asks for all of them at once. */
  probeRemote(host: string): Promise<RemoteProbe>;
  // The host book's folder list, written one entry at a time. The settings page
  // replaces a whole row; a control that knows one folder must not, or it sends
  // back blanks for every field it never displayed.
  addRemoteWorkspace(host: string, path: string): Promise<void>;
  removeRemoteWorkspace(host: string, path: string): Promise<void>;
  saveRemoteHost(entry: RemoteHostEdit): Promise<void>;
  removeRemoteHost(name: string): Promise<void>;
  // ssh_config aliases this machine can already reach and the book has not
  // taken yet. Filling it by hand when the addresses are written down next
  // door is the step people skip.
  remoteCandidates(): Promise<string[]>;
  // Questions a connect stopped for. Nothing arrives here in a browser: the
  // kernel that would ask is the one this build cannot reach.
  onRemoteAsk(cb: (ask: RemoteAsk) => void): () => void;
  answerRemote(id: string, ok: boolean, text: string): void;
  pickFolder(): Promise<string | null>;
  // Absolute paths of every file dropped anywhere on the window. It belongs
  // here rather than on a pane because the window has one of it: the shell
  // registers its drop listener once and ignores a second call, so a per-pane
  // subscription would quietly serve whichever pane opened first. Which element
  // the drop landed on is decided in the page against the DOM — the shell can
  // only offer coordinates, and coordinates part ways with CSS pixels under an
  // interface zoom. A browser tab never learns a path; there this unsubscribes
  // from nothing and dropped bytes go to AgentPort.attach instead.
  onDroppedPaths(cb: (paths: string[]) => void): () => void;
  portFor(rt: RuntimeView): AgentPort;
}

type HubRefusal = { code?: string; error?: string; params?: Record<string, string | number> };

export class SseHub implements HubPort {
  // One port per pane, kept across renders: a fresh instance would resubscribe
  // the event stream on every parent render and drop frames in between.
  private readonly ports = new Map<string, AgentPort>();
  // What this machine's kernel says it can drive at once, learned from the
  // last list. The default matches the kernel's own until one arrives.
  private paneCeiling = 8;
  // Bindings the shell exposes are pane-independent; this reaches them without
  // pretending to belong to a runtime.
  private readonly shell = new SsePort();

  // A refusal carries a code, and it is the code that has words in the reader's
  // language. Flattening the body into the message threw that away: what
  // reached the screen was the raw JSON, or a filesystem error naming a path.
  //
  // Read once as text, the way SseHttp does: not every answer is an envelope —
  // http.Error writes its reason as plain text, and asking for JSON first threw
  // that account away and left "the request never reached the kernel" in front
  // of a user whose kernel had just explained itself at length.
  private static async fail(path: string, res: Response): Promise<never> {
    const raw = (await res.text().catch(() => "")).trim();
    let body: HubRefusal | null = null;
    try {
      const parsed: unknown = raw === "" ? null : JSON.parse(raw);
      if (parsed && typeof parsed === "object") body = parsed as HubRefusal;
    } catch {
      // Plain text, which is the whole message.
    }
    const detail = body?.error || raw.slice(0, 400);
    throw new HttpError(res.status, detail || `${path}: ${res.status}`, body ?? undefined, detail !== "");
  }

  private async get<T>(path: string): Promise<T> {
    const res = await fetch(path, { credentials: "same-origin" });
    if (!res.ok) await SseHub.fail(path, res);
    return (await res.json()) as T;
  }

  private async post<T>(path: string, body?: unknown): Promise<T> {
    const res = await fetch(path, {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!res.ok) await SseHub.fail(path, res);
    return res.status === 204 ? (undefined as T) : ((await res.json()) as T);
  }

  // The ceiling comes back on the response rather than being hardcoded here:
  // the machine's own [desktop] max_panes decides it, and a client guessing 8
  // greys out the button at the wrong count.
  async runtimes() {
    const res = await fetch("/runtimes", { credentials: "same-origin" });
    if (!res.ok) throw new Error(`/runtimes: ${res.status}`);
    const max = Number(res.headers.get("X-Panes-Max"));
    if (max > 0) this.paneCeiling = max;
    return (await res.json()) as RuntimeView[];
  }

  maxPanes() {
    return this.paneCeiling;
  }

  open(req: { root?: string; sessionPath?: string }) {
    return this.post<RuntimeView>("/runtimes", { root: req.root ?? "", sessionPath: req.sessionPath ?? "" });
  }

  async close(id: string) {
    await this.post<void>(`/runtimes/${encodeURIComponent(id)}/close`);
    this.ports.delete(id);
  }

  tree() {
    return this.get<TreeWorkspace[]>("/tree");
  }

  addWorkspace(path: string) {
    return this.post<TreeWorkspace>("/tree/workspaces", { path });
  }

  async removeWorkspace(path: string) {
    await this.post<void>("/tree/workspaces/remove", { path });
  }

  async removeSession(path: string) {
    await this.post<void>("/tree/sessions/remove", { path });
  }

  async renameSession(path: string, title: string) {
    await this.post<void>("/tree/sessions/rename", { path, title });
  }

  async remoteHosts() {
    const res = await fetch("/remotes", { credentials: "same-origin" });
    // Not an error to report: this kernel simply does not do remote panes.
    if (res.status === 501) return null;
    if (!res.ok) await SseHub.fail("/remotes", res);
    return (await res.json()) as RemoteHost[];
  }

  openRemote(req: { host: string; workspace?: string; sessionPath?: string }) {
    return this.post<RuntimeView>("/remotes/open", {
      host: req.host,
      workspace: req.workspace ?? "",
      sessionPath: req.sessionPath ?? "",
    });
  }

  async remoteTree(host: string) {
    const path = `/remotes/${encodeURIComponent(host)}/tree`;
    const res = await fetch(path, { credentials: "same-origin" });
    // Nothing open on that machine yet, which is a step to take rather than a
    // failure to report.
    if (res.status === 409) return null;
    if (!res.ok) await SseHub.fail(path, res);
    return (await res.json()) as TreeWorkspace[];
  }

  probeRemote(host: string) {
    return this.get<RemoteProbe>(`/remotes/${encodeURIComponent(host)}/probe`);
  }

  remoteDirs(host: string, path?: string) {
    const at = path ? `?path=${encodeURIComponent(path)}` : "";
    return this.get<RemoteListing>(`/remotes/${encodeURIComponent(host)}/dirs${at}`);
  }

  async addRemoteWorkspace(host: string, path: string) {
    await this.post<void>(`/remotes/${encodeURIComponent(host)}/workspaces`, { path });
  }

  async removeRemoteWorkspace(host: string, path: string) {
    await this.post<void>(`/remotes/${encodeURIComponent(host)}/workspaces/remove`, { path });
  }

  async saveRemoteHost(entry: RemoteHostEdit) {
    await this.post<void>("/remotes", entry);
  }

  async removeRemoteHost(name: string) {
    await this.post<void>("/remotes/remove", { name });
  }

  remoteCandidates() {
    return this.get<string[]>("/remotes/candidates");
  }

  onRemoteAsk(cb: (ask: RemoteAsk) => void) {
    return this.shell.onRemoteAsk(cb);
  }

  answerRemote(id: string, ok: boolean, text: string) {
    this.shell.answerRemote(id, ok, text);
  }

  pickFolder() {
    return this.shell.pickFolder();
  }

  onDroppedPaths(cb: (paths: string[]) => void) {
    return this.shell.onDroppedPaths(cb);
  }

  portFor(rt: RuntimeView): AgentPort {
    const held = this.ports.get(rt.id);
    if (held) return held;
    const port = new SsePort(rt.base, rt.id);
    this.ports.set(rt.id, port);
    return port;
  }
}
