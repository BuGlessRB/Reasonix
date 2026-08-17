import { HttpError, type AgentPort } from "./port";
import { SsePort } from "./sse";

// RuntimeView is one open pane. Base is the prefix every request of that pane
// carries, which is also what tells the shell's bus which channel to listen on.
export interface RuntimeView {
  id: string;
  base: string;
  root: string;
  name: string;
  sessionPath?: string;
}

export interface TreeSession {
  path: string;
  name: string;
  title?: string;
  turns?: number;
  // Set when a pane already drives this transcript: the sidebar focuses that
  // pane rather than opening a second writer for one file.
  runtimeId?: string;
}

export interface TreeWorkspace {
  root: string;
  name: string;
  // A delivery worktree carries the project's own folder name, so the sidebar
  // needs this to tell the copy from the original.
  isolated?: boolean;
  missing?: boolean;
  open?: boolean;
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
  pickFolder(): Promise<string | null>;
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
    throw new HttpError(res.status, body?.error || `${path}: ${res.status}`, body ?? undefined);
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

  pickFolder() {
    return this.shell.pickFolder();
  }

  portFor(rt: RuntimeView): AgentPort {
    const held = this.ports.get(rt.id);
    if (held) return held;
    const port = new SsePort(rt.base, rt.id);
    this.ports.set(rt.id, port);
    return port;
  }
}
