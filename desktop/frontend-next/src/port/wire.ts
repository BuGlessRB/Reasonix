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
  startedAt?: number;
  endedAt?: number;
  partial?: boolean;
  parentId?: string;
  attemptId?: string;
  diff?: string;
  added?: number;
  removed?: number;
  profile?: Profile;
  execution?: Execution;
}

export interface CacheDiagnostics {
  prefixHash?: string;
  prefixChanged?: boolean;
  prefixChangeReasons?: string[];
  toolSchemaTokens?: number;
  cacheMissTokens?: number;
  cacheHitTokens?: number;
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

export interface Compaction {
  trigger?: string;
  messages?: number;
  summary?: string;
  archive?: string;
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
  constraint_degraded: boolean;
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
  compaction?: Compaction;
  streamAttempt?: StreamAttempt;
  completion?: CompletionSummary;
  err?: string;
  outcome?: string;
  phase?: string;
  retryAttempt?: number;
  retryMax?: number;
  retryScope?: "headers" | "stream";
  itemId?: string;
}
