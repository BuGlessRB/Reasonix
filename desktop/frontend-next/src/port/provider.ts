// provider.ts — a model source as the connection panel edits it: what was
// probed, what was confirmed, and the few fields no probe can answer.

export interface ProviderEntry {
  name: string;
  kind: string;
  baseUrl: string;
  models: string[];
  default: string;
  hasKey: boolean;
  keyEnv?: string;
  // Which of them read images, so an editor shows the current answer rather
  // than asking the user to remember it.
  visionModels?: string[];
  // False where the kernel refuses image input for this endpoint regardless of
  // config, so an editor can say so instead of offering a dead switch.
  canSetVision?: boolean;
  // The endpoint-executed search tool. canWebSearch says this door offers one
  // at all; webSearch whether it is on. They differ between an account's doors.
  canWebSearch?: boolean;
  webSearch?: boolean;
  // Whether thinking/reasoning_effort may go on the wire. canSetThinking is
  // false where the protocol never carries them, so the switch appears only
  // where a relay can actually reject the request over it.
  canSetThinking?: boolean;
  sendsThinking?: boolean;
  // Which request shape this endpoint controls thinking with, as declared.
  // Absent is "nobody said", which is why a relay's effort ladder comes out
  // empty — nothing can be probed for it.
  reasoningProtocol?: string;
  // Removing the one in use would leave the session on a model that no longer
  // resolves, so the row offers no delete.
  inUse: boolean;
  preset: boolean;
  // The three no probe can answer: the window is a fact about the model that
  // endpoints do not report, and headers / extra body are what a relay demands
  // on top of the protocol.
  contextWindow?: number;
  headers?: Record<string, string>;
  extraBody?: Record<string, unknown>;
}

// One wire format a source may be saved as, as the kernel declares it. The
// list is the kernel's and the words for it are ours: no label rides along.
export interface Protocol {
  kind: string;
  // Which model-listing shape this wire is discovered under. Protocols sharing
  // one value answer the same listing and cannot be told apart by a probe.
  discovery: string;
  // Whether the wire has a format for a provider-executed web search tool, and
  // whether thinking/effort fields ride it at all.
  serverWebSearch: boolean;
  reasoningParams: boolean;
}

// What an endpoint turned out to be. Every field is a guess the user confirms
// before anything is written — a model list cannot prove which protocol a
// gateway speaks, only which ones it answers.
export interface ProviderProbe {
  kind: string;
  // Every kind that listing may be driven with, kernel order. `kind` is the
  // pre-selection among them, not the only answer: DeepSeek serves both the
  // OpenAI chat wire and the Responses API off one model list.
  kinds: string[];
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

// What re-probing a saved provider found. `error` carries the endpoint's own
// words, because "401" and "no chat models" send the user to different fixes.
export interface ProviderCheck {
  ok: boolean;
  kind?: string;
  // Whether that answer is consistent with the kind the entry records. A
  // Responses source answers the OpenAI listing, so equality is the wrong test.
  matches?: boolean;
  models?: string[];
  ambiguous?: boolean;
  noProxy?: boolean;
  error?: string;
}

// Changing a source that already exists: everything else on the entry stays.
export interface ProviderEdit {
  name: string;
  baseUrl?: string;
  // Empty keeps the stored key.
  apiKey?: string;
  models: string[];
  default: string;
  vision: string[];
  // Omitted means "leave it alone" — sending an empty object instead would
  // clear headers a gateway needs. 0 is a real window: it turns automatic
  // compaction off for this source.
  contextWindow?: number;
  headers?: Record<string, string>;
  extraBody?: Record<string, unknown>;
  // Which request shape controls thinking here. "" is auto — no declaration,
  // which leaves the registry in charge. Omitted still means "leave it alone".
  reasoningProtocol?: string;
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

// What the opening sequence still owes a machine with no usable key. GET
// /provider-setup 404s once one exists, so null means "ready".
export interface ProviderSetup {
  required: boolean;
  provider?: string;
  model?: string;
  modelRef?: string;
  keyEnv?: string;
  error?: string;
  activationPending?: boolean;
}
