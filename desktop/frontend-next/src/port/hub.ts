import { HttpError, type AgentPort } from "./port";
import type { RemoteHost } from "./remote";
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
  openRemote(req: { host: string; workspace?: string }): Promise<RuntimeView>;
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
  private static async fail(path: string, res: Response): Promise<never> {
    const body = (await res.json().catch(() => null)) as
      | { code?: string; error?: string; params?: Record<string, string | number> }
      | null;
    throw new HttpError(res.status, body?.error || `${path}: ${res.status}`, body ?? undefined, !!body?.error);
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

  openRemote(req: { host: string; workspace?: string }) {
    return this.post<RuntimeView>("/remotes/open", { host: req.host, workspace: req.workspace ?? "" });
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
