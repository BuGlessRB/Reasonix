// What a machine's link is doing, in the kernel's own words. A host nobody has
// connected to reports "idle" rather than being absent — its row is where
// connecting starts. "degraded" is reachable but with a forward that did not
// attach, which reads as connected right up until a button does nothing.
export type RemoteStatus = "idle" | "connecting" | "connected" | "reconnecting" | "degraded" | "stopped";

// One machine in the host book. Everything past `target` is the live link, so a
// row drawn from configuration alone is complete and simply idle.
export interface RemoteHost {
  name: string;
  // How the user would type it for ssh, so a row is recognisable without
  // opening it: user@host, and a port only when it is not the assumed one.
  target: string;
  workspace?: string;
  status: RemoteStatus;
  // Which reconnect this is, while reconnecting.
  attempt?: number;
  // The bootstrap step a first connect has reached. Present only while one is
  // running: a cold connect installs a binary, and a spinner cannot say so.
  step?: string;
  detail?: string;
  error?: string;
  panes?: number;
}

// The bootstrap's steps, in the order a cold connect walks them. The kernel
// sends the key; the labels are here because only a screen needs words. An
// unknown key is shown as itself rather than dropped — a new step is worth
// seeing untranslated, and better than a list that silently skips it.
export const REMOTE_STEPS = ["detect", "install", "waiting_lock", "launch", "health_check", "ready", "reuse"] as const;

export const REMOTE_STEP_LABEL: Record<string, string> = {
  detect: "探测平台",
  install: "安装 reasonix",
  waiting_lock: "等待其他连接",
  launch: "启动 serve",
  health_check: "等它就绪",
  ready: "挂载端口转发",
  reuse: "接上已在运行的",
};
