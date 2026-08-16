import type { ProviderEdit, ProviderEntry, ProviderProbe } from "./port";
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

  async probeProvider(): Promise<ProviderProbe> {
    throw new Error("演示模式不会真的去连端点");
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
