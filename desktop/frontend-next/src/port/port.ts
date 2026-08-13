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
  sessions(): Promise<SessionEntry[]>;
  resume(path: string): Promise<void>;
  newSession(): Promise<void>;
  deleteSession(name: string): Promise<void>;
  status(): Promise<SessionStatus>;
  history(): Promise<HistoryMessage[]>;
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
