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

// GET /mcp. One external tool provider. state is ready | connecting | failed |
// disabled | idle: disabled is switched off and stays off across restarts, idle
// is configured and simply not needed yet. They look identical to the live host
// and mean opposite things, so the server resolves which one it is.
export interface McpEntry {
  name: string;
  state: string;
  enabled: boolean;
  transport?: string;
  source?: string;
  tools: number;
  prompts?: number;
  resources?: number;
  toolNames?: string[];
  error?: string;
}

// One server a paste resolved to, before anything has been installed.
export interface McpDraftServer {
  name: string;
  transport: string;
  command?: string;
  args?: string[];
  url?: string;
  env?: Record<string, string>;
  headers?: Record<string, string>;
}

// What the confirmation card must show: shell is the command that will run,
// unknown-host the endpoint it will talk to, secret a credential written out in
// full. None of them block the install — they are what the user is agreeing to.
export interface McpRisk {
  server: string;
  kind: "secret" | "shell" | "unknown-host";
  field: string;
  detail: string;
}

export interface McpDraft {
  servers: McpDraftServer[];
  risks: McpRisk[];
}

// state is ready | action_required | issue. action_required means the config was
// kept because finishing OAuth is impossible once the entry is gone.
export interface McpInstallResult {
  name: string;
  state: string;
  toolCount: number;
  action: string;
  message: string;
}

// One entry of GET /skills — every skill that may run, which is a larger set
// than /slash: a skill with no slash name still fires on model discovery.
export interface SkillEntry {
  name: string;
  slashName?: string;
  description?: string;
  scope?: string;
  plugin?: string;
  path?: string;
  subagent?: boolean;
  readOnly?: boolean;
  model?: string;
  effort?: string;
  allowedTools?: string[];
  manual?: boolean;
  enabled: boolean;
}

// implicit is the session-wide switch for model-initiated discovery. With it
// off every "auto" skill is manual in practice, so the rows have to say so.
export interface SkillCatalog {
  implicit: boolean;
  skills: SkillEntry[];
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

export interface ModelPrice {
  input: number;
  output: number;
  cacheHit?: number;
  currency?: string;
}

export interface ModelEntry {
  ref: string;
  provider: string;
  model: string;
  kind?: string;
  active?: boolean;
  default?: boolean;
  // The endpoint host. Rows sharing it are one service reached under more than
  // one protocol, which is what lets the list fold them into a single choice.
  vendor?: string;
  // What the kernel will actually do with this model. An absent field means
  // nothing declares it — never "no". Rendering a guess here sends the user to
  // a rejected request they cannot explain.
  vision?: boolean;
  efforts?: string[];
  effort?: string;
  contextWindow?: number;
  price?: ModelPrice;
}

export interface AccountUser {
  handle: string;
  email: string;
  label: string;
}

// signedIn with an error means the token is still here but the identity service
// could not be reached — never the same thing as being signed out.
export interface AccountState {
  signedIn: boolean;
  user?: AccountUser;
  expired?: boolean;
  error?: string;
}

export interface DeviceGrant {
  deviceCode: string;
  userCode: string;
  verificationUri: string;
  verificationUriComplete: string;
  interval: number;
  expiresIn: number;
}

export interface VersionEntry {
  version: string;
  tag: string;
  publishedAt: string;
  current: boolean;
  older: boolean;
}

// err rides alongside the data: an unreachable catalog must not hide which
// version is running.
export interface VersionHub {
  current: string;
  pinned: string;
  stalePin: boolean;
  latest: string;
  newer: boolean;
  versions: VersionEntry[];
  err?: string;
}

// A configured provider as the settings panel lists it.
// Which model takes which job. Only the roles with a field behind them in the
// kernel appear here; image routing joins once it can name its own model
// instead of borrowing whatever the subagent runs.
export interface RoleAssignments {
  planner: string;
  subagent: string;
  guardian: string;
}

export interface ProviderEntry {
  name: string;
  kind: string;
  baseUrl: string;
  models: string[];
  default: string;
  hasKey: boolean;
  // Removing the one in use would leave the session on a model that no longer
  // resolves, so the row offers no delete.
  inUse: boolean;
  preset: boolean;
}

// What an endpoint turned out to be. Every field is a guess the user confirms
// before anything is written — a model list cannot prove which protocol a
// gateway speaks, only which ones it answers.
export interface ProviderProbe {
  kind: string;
  authHeader: boolean;
  models: string[];
  default: string;
  efforts: string[];
  effort: string;
  vision: string[];
  // ambiguous: more than one protocol answered, so the kind is a preference
  // rather than a finding.
  ambiguous: boolean;
  // noProxy: it answered only with the proxy bypassed (a China-only endpoint
  // behind a foreign exit resets the handshake).
  noProxy: boolean;
}

// What the panel sends back after the user has looked at the probe.
export interface ProviderDraft {
  name: string;
  kind: string;
  baseUrl: string;
  apiKey: string;
  models: string[];
  default: string;
  authHeader: boolean;
  noProxy: boolean;
  effort: string;
  vision: string[];
}

// One report from an install in flight. received/total are meaningful only
// while downloading; verifying is the pause after the last byte, which is long
// enough on a large artifact that not naming it reads as a hang.
export interface UpdateProgress {
  version: string;
  phase: "downloading" | "verifying" | "downloaded" | "relaunching" | "error";
  received: number;
  total: number;
  err?: string;
}

export interface AgentPort {
  providerSetup(): Promise<ProviderSetup | null>;
  saveProviderKey(apiKey: string): Promise<void>;
  models(): Promise<ModelEntry[]>;
  slash(): Promise<SlashEntry[]>;
  skills(): Promise<SkillCatalog>;
  // Persisted, but the running session keeps the prompt index it was built
  // with: the switch reaches the model on the next rebuild, not this turn.
  setSkillEnabled(name: string, enabled: boolean): Promise<void>;
  hooks(): Promise<HookCatalog>;
  // Replaces one scope wholesale: a client that merges partial edits wrong
  // silently drops somebody else's rule.
  saveHooks(scope: "user" | "project", hooks: HookEntry[]): Promise<void>;
  dryRunHook(h: HookEntry): Promise<HookDryRun>;
  memories(): Promise<MemoryCatalog>;
  // Archives rather than deletes: a fact dropped by mistake stays recoverable.
  forgetMemory(name: string): Promise<void>;
  network(): Promise<NetworkSettings>;
  // password empty keeps whatever is stored; clearPassword removes it.
  saveNetwork(s: NetworkSettings, password: string, clearPassword: boolean): Promise<NetworkSettings>;
  diagnoseNetwork(): Promise<NetworkProbe[]>;
  mcp(): Promise<McpEntry[]>;
  // Retries a failed or disconnected server and answers with its new state, so
  // the caller never has to race a follow-up GET against the connect.
  reconnectMcp(name: string): Promise<{ state: string; tools?: number; error?: string }>;
  setMcpEnabled(name: string, enabled: boolean): Promise<void>;
  // Resolves a pasted block without touching anything. Separate from install so
  // the user sees what would run before agreeing to it.
  parseMcp(input: string): Promise<McpDraft>;
  installMcp(server: McpDraftServer, scope: "user" | "project"): Promise<McpInstallResult>;
  removeMcp(name: string): Promise<{ disconnected: boolean; stillConfigured: boolean }>;
  // An account is only for the networked surfaces (forum, crash follow-ups);
  // nothing in the agent loop calls these.
  // Updating the app is the shell's job, not the kernel's: only the shell knows
  // its install layout. A browser tab has no shell and gets an empty hub.
  // Empty means the job rides the main model — the default for every role, and
  // a real answer rather than a missing one.
  roles(): Promise<RoleAssignments>;
  // Persisted, then the runtime is rebuilt: boot reads every role model while
  // assembling, so an assignment cannot reach a runtime that is already up.
  setRole(role: string, ref: string): Promise<void>;
  providers(): Promise<ProviderEntry[]>;
  // Asks an endpoint what it is. Writes nothing — the answer is shown for
  // confirmation, because only the person holding the key knows what they
  // bought.
  probeProvider(baseUrl: string, apiKey: string): Promise<ProviderProbe>;
  saveProvider(draft: ProviderDraft): Promise<void>;
  removeProvider(name: string): Promise<void>;
  versions(): Promise<VersionHub>;
  pinVersion(version: string): Promise<void>;
  // Installs a published version, forward or back — the same call either way,
  // because a rollback that took a second code path would be the less-tested
  // one. Resolves only on failure: a success ends with the process handing over
  // to the build it just installed.
  goToVersion(version: string): Promise<void>;
  // Returns an unsubscribe. A browser tab has no shell to report progress, so
  // it never fires there.
  onUpdateProgress(cb: (p: UpdateProgress) => void): () => void;
  account(): Promise<AccountState>;
  accountLogin(): Promise<DeviceGrant>;
  accountPoll(deviceCode: string): Promise<{ status: "pending" | "complete"; slowDown?: boolean }>;
  accountLogout(): Promise<void>;
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
  // What the working tree actually differs by. Tool events cannot answer it: a
  // file created and then removed by a shell command leaves both events behind
  // and nothing on disk.
  changes(): Promise<WorkspaceChanges>;
  // Saves pasted or dropped image bytes into the workspace's attachment
  // directory and returns the "@path" token a turn references it by.
  attach(blob: Blob): Promise<Attachment>;
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

// One configured hook rule. blocking/usesMatch are the kernel's answer about
// the event, not the client's guess: whether exit 2 stops the agent is a
// property of the event, and issues are what Inspect already found wrong.
export interface HookEntry {
  event: string;
  match?: string;
  command: string;
  description?: string;
  timeout?: number;
  cwd?: string;
  scope: string;
  source?: string;
  blocking?: boolean;
  usesMatch?: boolean;
  readOnly?: boolean;
  issues?: string[];
}

export interface HookSource {
  scope: string;
  path: string;
  status: string;
  hookCount: number;
  parseError?: string;
}

export interface HookEventInfo {
  name: string;
  blocking: boolean;
  usesMatch: boolean;
}

// projectPath is empty when no project is open — project-scoped rules have
// nowhere to live, and the UI must not offer that scope.
export interface HookCatalog {
  hooks: HookEntry[];
  sources: HookSource[];
  events: HookEventInfo[];
  projectPath: string;
  globalPath: string;
}

// A real execution against a sample payload. blocks is the consequence on this
// event specifically — the same exit code stops one event and warns on another.
export interface HookDryRun {
  decision: string;
  exitCode: number;
  stdout?: string;
  stderr?: string;
  timedOut?: boolean;
  durationMs: number;
  blocks: boolean;
}

// Proxy settings as the editor needs them. The stored password never comes back
// out — hasPassword only says one exists, so the form can keep or clear it.
export interface NetworkSettings {
  mode: string;
  url?: string;
  noProxy?: string;
  type?: string;
  server?: string;
  port?: number;
  username?: string;
  hasPassword?: boolean;
  effective: string;
  direct?: string[];
  endpoint?: string;
}

// One diagnosed step. advice is the next thing to try, present only when the
// cause is knowable from the failure.
export interface NetworkProbe {
  step: string;
  ok: boolean;
  detail: string;
  durationMs: number;
  advice?: string;
}

// One saved fact. activation is the field a reader needs most and the type does
// not imply it: pinned facts sit in every prompt, relevant ones only surface
// when the turn looks related. usedLastTurn ties a stored fact to the behaviour
// the user just watched — the question they actually have.
export interface MemoryEntry {
  name: string;
  title?: string;
  description?: string;
  body?: string;
  type?: string;
  scope?: string;
  activation: string;
  path?: string;
  revision?: number;
  createdAt?: string;
  updatedAt?: string;
  expired?: boolean;
  usedLastTurn?: boolean;
  why?: string;
}

export interface MemoryCatalog {
  memories: MemoryEntry[];
  recallQuery: string;
  indexPath?: string;
}
