import type { ProviderView } from "./types";

export type ProviderModelVisionCapability = "supported" | "unsupported" | "unknown";

/**
 * Returns the models known to accept images, merging model metadata with the
 * legacy provider-level list without allowing a stale legacy entry to override
 * an explicit per-model false value.
 */
export function providerVisionModelsForView(
  provider: Pick<ProviderView, "models" | "visionModels" | "modelOverrides">,
  models = provider.models,
): string[] {
  const legacyVision = new Set(provider.visionModels);
  const overrides = new Map((provider.modelOverrides ?? []).map((override) => [override.model.trim().toLowerCase(), override.vision]));
  return models.filter((model) => {
    const override = overrides.get(model.trim().toLowerCase());
    if (override !== undefined && override !== null) return override;
    return legacyVision.has(model);
  });
}

/** Returns a conservative read-only capability label for one model. */
export function providerModelVisionCapability(
  provider: Pick<ProviderView, "visionModelsConfigured">,
  model: string,
  visionModels: string[],
): ProviderModelVisionCapability {
  if (visionModels.includes(model)) return "supported";
  return provider.visionModelsConfigured ? "unsupported" : "unknown";
}
