// Mirrors internal/eventwire. Field names and kind strings must match exactly;
// this file is the contract, not a convenience shape.

export type Kind =
  | "turn_started"
  | "reasoning"
  | "text"
  | "message"
  | "tool_dispatch"
  | "tool_result"
  | "tool_progress"
  | "usage"
  | "notice"
  | "phase"
  | "turn_phase"
  | "approval_request"
  | "ask_request"
  | "turn_done"
  | "compaction_started"
  | "compaction_progress"
  | "compaction_done"
  | "mcp_surface_ready"
  | "retrying"
  | "steer"
  | "guardian_assessment"
  | "extension_surface"
  | "extension_status"
  | "stream_attempt"
  | "context_maintenance"
  | "workspace_changed"
  | "completion_summary";

export interface Profile {
  name?: string;
  description?: string;
}

// Present on shell tools. A non-zero exit was invisible before: the card only
// ever rendered stdout, so a command that failed looked like one that ran.
export interface Execution {
  kind?: string;
  shell?: string;
  platform?: string;
  state?: string;
  failurePhase?: string;
  exitCode?: number;
  outputTail?: string;
  mutationRisk?: string;
  verification?: string;
  durationMs?: number;
  contextTokens?: number;
}

export interface Tool {
  id?: string;
  name: string;
  args?: string;
  resolvedName?: string;
  capabilityId?: string;
  output?: string;
  err?: string;
  readOnly: boolean;
  truncated?: boolean;
  durationMs?: number;
  contextTokens?: number;
  startedAt?: number;
  endedAt?: number;
  partial?: boolean;
  // Cumulative argument characters received so far on a partial dispatch — the
  // only liveness a streaming payload has before its JSON parses.
  argChars?: number;
  refreshed?: boolean;
  parentId?: string;
  attemptId?: string;
  diff?: string;
  added?: number;
  removed?: number;
  profile?: Profile;
  execution?: Execution;
}

// Only prefixChangeReasons is omitempty on the wire; the rest always arrive, so
// marking them optional would make every reader handle an absence that the
// producer never sends.
export interface CacheDiagnostics {
  prefixHash: string;
  prefixChanged: boolean;
  prefixChangeReasons?: string[];
  toolSchemaTokens: number;
  cacheMissTokens: number;
  cacheHitTokens: number;
}

export interface Money {
  amount: string;
  currency: string;
}

export interface CostQuote {
  original: Money;
  selected?: Money;
  estimated: boolean;
  costComplete: boolean;
}

export interface Usage {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  cacheHitTokens: number;
  cacheMissTokens: number;
  reasoningTokens?: number;
  estimated?: boolean;
  source?: string;
  cacheDiagnostics?: CacheDiagnostics;
  sessionCacheHitTokens: number;
  sessionCacheMissTokens: number;
  // Context* is the latest single request's shape, for gauges and rebind. Absent
  // means fall back to the billable prompt/completion totals above.
  contextPromptTokens?: number;
  contextCompletionTokens?: number;
  contextReasoningTokens?: number;
  contextCacheHitTokens?: number;
  contextCacheMissTokens?: number;
  cost?: number;
  currency?: string;
  currencyCode?: string;
  costQuote?: CostQuote;
}

export interface Approval {
  id: string;
  tool: string;
  subject: string;
  reason?: string;
  fresh?: boolean;
  kind?: "tool" | "plan" | "recovery";
}

export interface AskOption {
  label: string;
  description?: string;
}

// No json tags on event.AskAnswer, so the wire keys are the Go field names.
export interface AskAnswer {
  QuestionID: string;
  Selected: string[];
}

export interface AskQuestion {
  id: string;
  header?: string;
  prompt: string;
  options: AskOption[];
  multi?: boolean;
}

export interface Ask {
  id: string;
  questions: AskQuestion[];
}

export interface Guardian {
  id: string;
  tool: string;
  subject: string;
  outcome: string;
  risk_level?: string;
  user_authorization?: string;
  rationale?: string;
  duration_ms?: number;
}

// Extension UI surfaces. A sidecar publishes data, never markup — the host
// decides how to draw it, which is why the same extension shows up as a card
// here, as text in the CLI, and as a client-rendered block over ACP.
export interface ExtensionStatus {
  label: string;
  detail?: string;
  severity?: string;
  progress?: number;
}

export interface ExtensionKeyValue {
  key: string;
  value: string;
}

export interface ExtensionActionRef {
  actionId: string;
  label: string;
}

export interface ExtensionCard {
  title?: string;
  markdown?: string;
  text?: string;
  fields?: ExtensionKeyValue[];
  progress?: number;
  actions?: ExtensionActionRef[];
}

export interface ExtensionFormField {
  key: string;
  label?: string;
  kind?: "confirm" | "input" | "select" | "multiselect";
  options?: string[];
  default?: unknown;
  required?: boolean;
}

export interface ExtensionForm {
  title?: string;
  message?: string;
  fields: ExtensionFormField[];
}

// A panel has no markdown on purpose — see the protocol DTO: the side rail is
// a narrow column, and a rendered document there costs more than it tells.
export interface ExtensionPanel {
  title?: string;
  text?: string;
  fields?: ExtensionKeyValue[];
  progress?: number;
  actions?: ExtensionActionRef[];
}

export interface ExtensionNotification {
  title: string;
  body?: string;
  severity?: string;
}

// A view is composed rather than filled in: the extension sends a tree of
// primitives and this side renders them with its own components. Tone says
// what a node means, never what colour it is — the palette stays ours.
export type ExtensionViewTone = "dim" | "strong" | "ok" | "warn" | "err" | "accent";

export interface ExtensionViewNode {
  kind: "text" | "markdown" | "row" | "stack" | "kv" | "meter" | "pip" | "button" | "divider";
  value?: string;
  key?: string;
  label?: string;
  tone?: ExtensionViewTone;
  progress?: number;
  actionId?: string;
  children?: ExtensionViewNode[];
}

export interface ExtensionView {
  // Where the extension would like to stand. A name we do not know renders
  // where we put views we have no place for, rather than not at all.
  slot?: string;
  // "tool:<callId>" when this view replaces a card's body instead of standing
  // on its own. Only tool calls can be anchored — an approval prompt or an
  // error state is not addressable, which is what keeps a takeover from being
  // able to redraw a decision.
  anchor?: string;
  body: ExtensionViewNode[];
}

export interface ExtensionSurface {
  pluginId: string;
  surfaceId: string;
  sessionId?: string;
  generation?: number;
  kind: "status" | "card" | "form" | "notification" | "panel" | "view";
  status?: ExtensionStatus;
  card?: ExtensionCard;
  form?: ExtensionForm;
  notification?: ExtensionNotification;
  panel?: ExtensionPanel;
  view?: ExtensionView;
}

export interface Compaction {
  trigger?: string;
  messages?: number;
  summary?: string;
  // What the fold cost, and what the digest kept of it. coverageRequired counts
  // the changes and failures the folded work produced; coverageMissing is how
  // many the digest did not carry.
  sourceTokens?: number;
  projectionTokens?: number;
  coverageRequired?: number;
  coverageMissing?: number;
  coverageRepaired?: boolean;
}

export interface StreamAttempt {
  id: string;
  action: "begin" | "discard" | "commit";
  attempt?: number;
  max?: number;
  reason?: string;
}

export interface CompletionSummary {
  preset: string;
  verdict: string;
  mutations: number;
  checks_passed: number;
  checks_failed: number;
  checks_suppressed: number;
  review: string;
  gap_kinds?: string[];
}

// MemoryCitation is one local memory the turn drew on, so an answer can show
// what it was grounded in rather than asserting it.
export interface MemoryCitation {
  id?: string;
  source: string;
  lineStart?: number;
  lineEnd?: number;
  note?: string;
  kind?: string;
}

export interface WireEvent {
  kind: Kind;
  text?: string;
  detail?: string;
  code?: string;
  reasoning?: string;
  level?: string;
  tool?: Tool;
  usage?: Usage;
  approval?: Approval;
  ask?: Ask;
  guardian?: Guardian;
  extension?: ExtensionSurface;
  compaction?: Compaction;
  streamAttempt?: StreamAttempt;
  completion?: CompletionSummary;
  err?: string;
  memoryCitations?: MemoryCitation[];
  // The turn a compaction checkpoint committed under, so a reader can tell a
  // fold apart from the turns around it.
  checkpointTurn?: number;
  // recovery_paused is no longer emitted; sessions recorded before the retry
  // budgets were removed still carry it, so a reader has to render it.
  outcome?: "final_readiness" | "recovery_paused";
  phase?: string;
  retryAttempt?: number;
  retryMax?: number;
  retryScope?: "headers" | "stream";
  itemId?: string;
}
