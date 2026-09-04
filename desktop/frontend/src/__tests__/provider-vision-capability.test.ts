// Run: tsx src/__tests__/provider-vision-capability.test.ts

import type { ProviderView } from "../lib/types";
import { providerModelVisionCapability, providerVisionModelsForView } from "../lib/providerVisionCapability";

let passed = 0;
let failed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

function provider(overrides: Partial<ProviderView> = {}): ProviderView {
  return {
    name: "custom",
    builtIn: false,
    added: true,
    kind: "openai",
    baseUrl: "https://example.test/v1",
    modelsUrl: "",
    models: ["vision-model", "text-model", "opaque-model"],
    visionModels: ["vision-model"],
    visionModelsConfigured: true,
    default: "vision-model",
    apiKeyEnv: "CUSTOM_API_KEY",
    keySet: true,
    balanceUrl: "",
    contextWindow: 0,
    reasoningProtocol: "",
    thinking: "",
    supportedEfforts: [],
    defaultEffort: "",
    ...overrides,
  };
}

const legacy = provider({
  modelOverrides: [{
    model: "vision-model",
    reasoningProtocol: "",
    supportedEfforts: [],
    defaultEffort: "",
    vision: false,
  }],
});
ok(
  providerVisionModelsForView(legacy).length === 0,
  "model override can disable a legacy vision list entry",
);

const metadata = provider({
  visionModels: [],
  visionModelsConfigured: false,
  modelOverrides: [{
    model: "vision-model",
    reasoningProtocol: "",
    supportedEfforts: [],
    defaultEffort: "",
    vision: true,
  }],
});
const metadataVision = providerVisionModelsForView(metadata);
ok(metadataVision.length === 1 && metadataVision[0] === "vision-model", "model metadata enables native vision without VisionModels");
ok(providerModelVisionCapability(metadata, "vision-model", metadataVision) === "supported", "supported model is marked read-only");
ok(providerModelVisionCapability(metadata, "opaque-model", metadataVision) === "unknown", "unknown model stays conservative");

process.stdout.write(`provider vision capability: ${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
