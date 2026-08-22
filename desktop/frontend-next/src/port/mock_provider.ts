import type { Protocol, ProviderCheck, ProviderEdit, ProviderEntry, ProviderProbe } from "./port";
import { MockBoundary } from "./mock_boundary";

// The sources half of the fixture. Chained on for the reason the others are:
// MockPort satisfies AgentPort in one declaration, and each face keeps its own
// file.
export class MockProvider extends MockBoundary {
  // Three shapes the account grouping has to keep apart: one vendor reached
  // under two protocols, a custom relay serving other vendors' models, and two
  // tenants of that same relay holding different keys.
  private sources: ProviderEntry[] = [
    {
      name: "deepseek", kind: "openai", baseUrl: "https://api.deepseek.com",
      models: ["deepseek-v4-pro", "deepseek-v4-flash"], default: "deepseek-v4-pro",
      hasKey: true, inUse: true, preset: false, keyEnv: "DEEPSEEK_API_KEY", canSetVision: false,
    },
    {
      name: "deepseek-anthropic", kind: "anthropic", baseUrl: "https://api.deepseek.com/anthropic",
      models: ["deepseek-v4-pro"], default: "deepseek-v4-pro",
      hasKey: true, inUse: false, preset: false, keyEnv: "DEEPSEEK_API_KEY",
      canSetVision: false, canWebSearch: true, webSearch: true,
      canSetThinking: true, sendsThinking: true,
    },
    {
      name: "myrelay", kind: "openai", baseUrl: "https://relay.example.com/v1",
      models: ["gpt-4o", "claude-sonnet-4"], default: "gpt-4o",
      hasKey: true, inUse: false, preset: false, keyEnv: "MYRELAY_API_KEY",
      visionModels: ["gpt-4o"], canSetVision: true,
    },
    {
      name: "myrelay-work", kind: "openai", baseUrl: "https://relay.example.com/v1",
      models: ["gpt-4o"], default: "gpt-4o",
      hasKey: true, inUse: false, preset: false, keyEnv: "MYRELAY_WORK_API_KEY",
    },
  ];

  async providers(): Promise<ProviderEntry[]> {
    return this.sources;
  }

  // Mirrors the kernel catalog: two wires answer one OpenAI model listing.
  async protocols(): Promise<Protocol[]> {
    return [
      { kind: "openai", discovery: "openai", serverWebSearch: false, reasoningParams: true },
      { kind: "responses", discovery: "openai", serverWebSearch: true, reasoningParams: false },
      { kind: "anthropic", discovery: "anthropic", serverWebSearch: true, reasoningParams: false },
    ];
  }

  // The opening sequence is the only way into the fixture, and it advances on
  // what the probe found: refusing here leaves the demo on its first screen
  // with no way past, so it answers with data like every other face does.
  async probeProvider(): Promise<ProviderProbe> {
    return {
      kind: "openai", kinds: ["openai", "responses"], authHeader: true,
      models: ["deepseek-v4-pro", "deepseek-v4-flash"], default: "deepseek-v4-pro",
      efforts: ["high", "max"], effort: "high", vision: [],
      ambiguous: true, noProxy: false,
    };
  }

  // The fixture's endpoints answer both protocols, which is the case the row's
  // "改用…" repair exists for. The relay answers at gateway scale, which is the
  // case the model list's search and its row cap exist for.
  async checkProvider(name: string): Promise<ProviderCheck> {
    if (name === "mimo") return { ok: false, error: "401 unauthorized: key 过期了" };
    const models = name.startsWith("myrelay") ? relayCatalog() : ["gpt-4o", "claude-sonnet-4"];
    return { ok: true, kind: "openai", models, ambiguous: true };
  }

  async saveProvider(): Promise<void> {}

  async setProviderWebSearch(name: string, on: boolean): Promise<void> {
    this.sources = this.sources.map((p) => (p.name === name ? { ...p, webSearch: on } : p));
  }

  async setProviderThinking(name: string, on: boolean): Promise<void> {
    this.sources = this.sources.map((p) => (p.name === name ? { ...p, sendsThinking: on } : p));
  }

  // The kernel refuses a null anywhere in the extra body — TOML cannot hold one,
  // so it would be dropped on save and the field would never reach the wire.
  async editProvider(edit: ProviderEdit): Promise<void> {
    if (edit.extraBody && JSON.stringify(edit.extraBody).includes("null")) {
      throw new Error("extra body field cannot be null");
    }
    this.sources = this.sources.map((p) =>
      p.name === edit.name
        ? {
            ...p, models: edit.models, default: edit.default, visionModels: edit.vision,
            contextWindow: edit.contextWindow, headers: edit.headers, extraBody: edit.extraBody,
          }
        : p,
    );
  }

  async removeProvider(): Promise<void> {}
}

// A gateway answering at the scale the connection panel has to survive: the
// families repeat under date suffixes, which is exactly what makes such a list
// unscannable by eye. Issue #9192 is one of these, with 216 entries.
function relayCatalog(): string[] {
  const families = [
    "qwen3-max", "qwen3-omni-flash", "qwen3-vl-plus", "qwen3-next-80b-a3b",
    "MiniMax-M2", "MiniMax/speech-02", "ZHIPU/GLM-5", "kimi-k2",
    "deepseek-v4-pro", "deepseek-v4-flash", "gpt-4o", "claude-sonnet-4",
    "gemini-3-pro", "llama-4-maverick", "mistral-large", "grok-4",
  ];
  const out: string[] = [];
  for (const f of families) {
    out.push(f);
    for (const s of ["preview", "realtime", "2025-09-15", "2025-12-01", "2026-01-23", "2026-02-23"]) {
      out.push(`${f}-${s}`);
    }
  }
  return out;
}
