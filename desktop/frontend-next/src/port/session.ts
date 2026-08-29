// A conversation and the states it passes through: what is running, what it
// cost, and the points it can be wound back to.
// GET /history: the event stream is live-only, so a reload rebuilds from these.
export interface HistoryMessage {
  role: "system" | "user" | "assistant" | "tool";
  content: string;
  reasoning?: string;
  images?: number; // attachments on a user turn; an image-only one has no text
  toolCalls?: { id: string; name: string; arguments?: string }[];
  toolCallId?: string;
  toolName?: string;
}

// GET /checkpoints as internal/serve writes it: the snapshot the kernel took
// before each turn's first write. prompt is the user's own text, with the
// compose prefixes already stripped kernel-side.
export interface Checkpoint {
  turn: number;
  prompt: string;
  files: number;
}

// Only edit-tool writes are snapshotted, so "code" restores those files and
// nothing else — a shell command's side effects are not recoverable.
export type RewindScope = "code" | "conversation" | "both";

// What POST /rewind/prepare answers: the plan the kernel would apply, plus what
// it cannot reach. requiresConfirmation is true when coverage is partial — the
// turn also changed things outside the snapshot, typically via bash.
// What POST /rewind/commit answers. transactionId is what undo needs, and
// undoAvailable says whether the kernel can still reverse it.
export interface RewindResult {
  ok?: boolean;
  transactionId?: string;
  undoAvailable?: boolean;
  deleted?: string[];
  written?: string[];
}

export interface RewindPlan {
  planId: string;
  turn: number;
  coverage: string;
  coverageGaps?: { reason: string; detail: string; tool?: string }[];
  canFiles: boolean;
  canConversation: boolean;
  disabledReason?: string;
  files?: string[];
  fileCount: number;
  requiresConfirmation: boolean;
  // Single-file revert only: which file, and whether it has moved since the
  // checkpoint captured it. A conflict is the one case the caller must answer.
  path?: string;
  conflicts?: RewindConflict[];
}

// Why one file cannot be put back without a decision.
export interface RewindConflict {
  path: string;
  reason: string;
  currentExisted?: boolean;
}

export type ApprovalMode = "ask" | "auto" | "dontAsk" | "yolo";

// "light" was retired into balanced: its only enforced differences were two
// sub-agent switches, and a setting that costs a choice without changing what
// it names is a question not worth asking. Old sessions still send it.
export type Preset = "light" | "balanced" | "delivery";

export type ApprovalVerdict = "once" | "always" | "deny";

// Shape of GET /status as internal/serve writes it. Anything the UI wants that
// is not here has to be added on the Go side, not invented in the client.
// PlanAction is what a plan card can answer. The three are distinct kernel
// transitions, not an allow/deny pair: revising keeps planning, exiting leaves
// the workflow, and only starting carries the plan into execution.
export type PlanAction = "start" | "revise" | "exit";

// PLAN_ACTIONS maps them to the names the kernel answers to.
export const PLAN_ACTIONS: Record<PlanAction, string> = {
  start: "start_execution",
  revise: "revise_plan",
  exit: "exit_plan",
};

// PlanPhase is where the run sits in the plan lifecycle. Absent from a status
// means outside it; there is deliberately no "inactive" string to render.
export type PlanPhase = "planning" | "awaiting_approval" | "executing";

// AskReason is why only the user can answer. The two values are the whole set:
// permission and plan approval are different decisions with different owners.
export type AskReason = "user_decision" | "missing_value";

export interface DecisionOption {
  label: string;
  description?: string;
}

export interface DecisionQuestion {
  id: string;
  header?: string;
  prompt: string;
  reason?: AskReason;
  options: DecisionOption[];
  multi?: boolean;
}

// A Decision is something waiting on the user, named by the id its owner
// issued. The id goes back untouched — the frontend never describes the state
// it thinks the run is in, and never resolves a decision itself: it disappears
// when the next projection no longer carries it.
export interface Decision {
  id: string;
  kind: "plan_approval" | "ask";
  questions?: DecisionQuestion[];
}

export interface SessionStatus {
  // Whether the current model reads images at all, and whether that answer was
  // ever given: a relay forwards models nothing here has a label for, and an
  // undeclared one is not the same claim as a text-only one.
  vision?: boolean;
  visionDeclared?: boolean;
  label: string;
  running: boolean;
  // plan drives the composer toggle and nothing else: it is the legacy
  // projection, false once a plan is approved. planPhase is the lifecycle, and
  // it is absent when the run is outside the workflow entirely.
  plan: boolean;
  planPhase?: PlanPhase;
  decisions?: Decision[];
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
  sessionCostQuote?: import("./wire").CostQuote;
  jobs?: JobEntry[];
}

/** One currency of a wallet, rendered where the symbol rules live. Two
 *  currencies are two lines and never a sum — combining them would mean
 *  inventing an exchange rate — so the only thing done here is stack them. */
export interface WalletLine {
  currency: string;
  total: string;
  granted?: string;
}

/** What the provider's wallet says, and when it said it. A value standing in
 *  past its freshness says so rather than looking current — the endpoint being
 *  briefly unreachable is not the account being empty. */
export interface WalletReading {
  display: string;
  available: boolean;
  stale: boolean;
  fetchedAt: string;
  lines?: WalletLine[];
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
export interface SessionEntry {
  name: string;
  path: string;
  title?: string;
  turns?: number;
  current?: boolean;
}
