import type { AgentPort } from "./port";
import type { HubPort, RuntimeView, TreeWorkspace } from "./hub";
import { MockPort } from "./mock";

// MockHub is the fixture's answer to a window that drives several panes. Each
// pane gets its own scripted session, so the split view can be worked on
// without a kernel — the dev build's only reason to exist.
export class MockHub implements HubPort {
  private readonly ports = new Map<string, AgentPort>();
  private readonly views: RuntimeView[] = [
    { id: "r1", base: "", root: "~/projects/DeepSeek-Reasonix", name: "DeepSeek-Reasonix", sessionPath: "/sessions/mock.jsonl" },
  ];
  private readonly roots = ["~/projects/DeepSeek-Reasonix", "~/projects/my-website"];
  private seq = 1;

  maxPanes() {
    return 8;
  }

  runtimes() {
    return Promise.resolve([...this.views]);
  }

  open(req: { root?: string; sessionPath?: string }) {
    const held = req.sessionPath ? this.views.find((v) => v.sessionPath === req.sessionPath) : undefined;
    if (held) return Promise.resolve(held);
    this.seq++;
    const root = req.root || this.roots[0];
    const view: RuntimeView = {
      id: `r${this.seq}`,
      base: `/rt/r${this.seq}`,
      root,
      name: root.split("/").pop() ?? root,
      sessionPath: req.sessionPath,
    };
    this.views.push(view);
    return Promise.resolve(view);
  }

  close(id: string) {
    const at = this.views.findIndex((v) => v.id === id);
    if (at >= 0) this.views.splice(at, 1);
    this.ports.delete(id);
    return Promise.resolve();
  }

  tree() {
    const open = new Map(this.views.filter((v) => v.sessionPath).map((v) => [v.sessionPath!, v.id]));
    return Promise.resolve(
      this.roots.map<TreeWorkspace>((root, i) => ({
        root,
        name: root.split("/").pop() ?? root,
        open: this.views.some((v) => v.root === root),
        remembered: true,
        sessions:
          i === 0
            ? [
                { path: "/sessions/mock.jsonl", name: "mock", title: "并行会话演示", turns: 3, runtimeId: open.get("/sessions/mock.jsonl") },
                { path: "/sessions/older.jsonl", name: "older", title: "上一次的会话", turns: 12 },
              ]
            : [{ path: "/sessions/site.jsonl", name: "site", title: "站点改版", turns: 5 }],
      })),
    );
  }

  addWorkspace(path: string) {
    if (!this.roots.includes(path)) this.roots.push(path);
    return Promise.resolve<TreeWorkspace>({ root: path, name: path.split("/").pop() ?? path, remembered: true, sessions: [] });
  }

  removeWorkspace(path: string) {
    const at = this.roots.indexOf(path);
    if (at >= 0) this.roots.splice(at, 1);
    return Promise.resolve();
  }

  removeSession(_path: string) {
    return Promise.resolve();
  }

  renameSession(_path: string, _title: string) {
    return Promise.resolve();
  }

  pickFolder() {
    return Promise.resolve<string | null>("~/projects/mock-workspace");
  }

  // The fixture has no shell to report a path, so a drop there carries bytes.
  onDroppedPaths(): () => void {
    return () => {};
  }

  portFor(rt: RuntimeView): AgentPort {
    const held = this.ports.get(rt.id);
    if (held) return held;
    const port = new MockPort();
    this.ports.set(rt.id, port);
    return port;
  }
}
