import type { WireEvent } from "./wire";

// GET /history returns the provider conversation, not the event stream: the
// stream is live-only, so a reload rebuilds the transcript from these.
export interface HistoryMessage {
  role: "system" | "user" | "assistant" | "tool";
  content: string;
  reasoning?: string;
  toolCalls?: { id: string; name: string; arguments?: string }[];
  toolCallId?: string;
  toolName?: string;
}

export type ApprovalMode = "ask" | "auto" | "dontAsk" | "yolo";
export type Preset = "light" | "balanced" | "delivery";
export type ApprovalVerdict = "once" | "always" | "deny";

// Shape of GET /status as internal/serve writes it. Anything the UI wants that
// is not here has to be added on the Go side, not invented in the client.
export interface SessionStatus {
  label: string;
  running: boolean;
  plan: boolean;
  preset: Preset;
  effort?: string;
  modelRef?: string;
  toolApprovalMode: ApprovalMode;
  autoApproveTools: boolean;
  bypass: boolean;
  goal: string;
  goalStatus: string;
  cwd: string;
  workspaceRoot?: string;
  sessionPath?: string;
  used: number;
  window: number;
  cacheHit: number;
  cacheMiss: number;
  balance?: { display: string; available: boolean };
  sessionCostQuote?: import("./wire").CostQuote;
  jobs?: JobEntry[];
}

export interface JobEntry {
  id: string;
  kind: string;
  label: string;
  status: string;
  startedAt: number;
}

// The UI depends on this and nothing else. SsePort talks to internal/serve;
// MockPort replays a fixture. Neither is allowed to leak transport details
// upward, which is what keeps the same UI usable in a browser and in Wails.
// GET /provider-setup 404s once a usable key exists, so null means "ready".
export interface ProviderSetup {
  required: boolean;
  provider?: string;
  model?: string;
  modelRef?: string;
  keyEnv?: string;
  error?: string;
  activationPending?: boolean;
}

export interface SessionEntry {
  name: string;
  path: string;
  title?: string;
  turns?: number;
  current?: boolean;
}

// GET /mcp. One external tool provider. state is ready | connecting | failed
// | idle, where idle means configured but not connected yet.
export interface McpEntry {
  name: string;
  state: string;
  transport?: string;
  source?: string;
  tools: number;
  prompts?: number;
  resources?: number;
  toolNames?: string[];
  error?: string;
}

export interface WorkspaceEntry {
  path: string;
  name: string;
}

// GET /workspaces. canSwitch is the server's answer, not the client's guess:
// a server reachable over the network refuses to be repointed at all.
export interface WorkspaceInfo {
  current: string;
  canSwitch: boolean;
  canIsolate: boolean;
  recents: WorkspaceEntry[];
  isolated?: boolean;
}

// One entry of GET /slash: everything Submit resolves after a "/". The kernel
// already dedupes and orders these, so the menu must not re-sort them.
export interface SlashEntry {
  name: string;
  kind: "skill" | "command";
  description?: string;
  argHint?: string;
  scope?: string;
  plugin?: string;
  subagent?: boolean;
}

export interface ModelEntry {
  ref: string;
  provider: string;
  model: string;
  active?: boolean;
  default?: boolean;
}

export interface AgentPort {
  providerSetup(): Promise<ProviderSetup | null>;
  saveProviderKey(apiKey: string): Promise<void>;
  models(): Promise<ModelEntry[]>;
  slash(): Promise<SlashEntry[]>;
  mcp(): Promise<McpEntry[]>;
  workspaces(): Promise<WorkspaceInfo>;
  // Rebuilds the whole runtime against another folder. The conversation does
  // not come along, so the caller has to reload the transcript afterwards.
  setWorkspace(path: string): Promise<void>;
  isolateWorkspace(): Promise<void>;
  // The native folder picker, or "" where there is none (a browser tab) or
  // when the user cancelled. Only the shell can open one.
  pickFolder(): Promise<string>;
  sessions(): Promise<SessionEntry[]>;
  resume(path: string): Promise<void>;
  newSession(): Promise<void>;
  deleteSession(name: string): Promise<void>;
  status(): Promise<SessionStatus>;
  history(): Promise<HistoryMessage[]>;
  // Replaying the persisted wire frames rebuilds the trajectory pane row for
  // row; the live stream only ever covers the current connection.
  trajectory(): Promise<WireEvent[]>;
  subscribe(onEvent: (ev: WireEvent) => void): () => void;

  submit(text: string): Promise<void>;
  // /submit 409s once a turn holds the session. Mid-turn input is durable and
  // goes through the inbox, which delivers it at the next tool boundary.
  steer(text: string): Promise<void>;
  cancel(): Promise<void>;
  approve(id: string, verdict: ApprovalVerdict): Promise<void>;
  answer(id: string, answers: { questionId: string; selected: string[] }[]): Promise<void>;

  setPlanMode(on: boolean): Promise<void>;
  setApprovalMode(mode: ApprovalMode): Promise<void>;
  setPreset(preset: Preset): Promise<void>;
  setModel(ref: string): Promise<void>;
  setEffort(effort: string): Promise<void>;
  setGoal(text: string): Promise<void>;
}
