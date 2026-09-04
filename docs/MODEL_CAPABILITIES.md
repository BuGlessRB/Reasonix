# Model capability metadata

Reasonix resolves input capabilities per model through the provider adapter.
Adapters return `inputModalities` for the exact model, following the
`deepseek-harness` model contract:

- `text` means text input is accepted.
- `image` means native image input is accepted.
- `text + image` enables native multimodal requests without a `VisionModels`
  setting.

OpenAI-compatible `/models` responses may use the canonical
`input_modalities` field. Reasonix also accepts `modalities.input`,
`capabilities.input_modalities`, `capabilities.vision`, `supports_vision`, and
`vision` as compatibility aliases. If no positive declaration is available,
the generic adapter defaults the exact model to text-only; it never infers
vision from a model name.

Dynamic metadata is stored in the disposable
`model-capabilities-v1.json` cache under the Reasonix cache directory. It is
not written to `config.toml`. Existing `vision` and `vision_models` entries
remain readable for backwards compatibility and take precedence over dynamic
metadata.

Built-in adapters also ship verified local catalogs for the official OpenCode
Go routes, the DeepSeek vision SKU, and the curated ModelScope Qwen3.5 SKUs.
These catalogs work without a model-list request; a custom endpoint or a model
not in a local catalog still uses the safe text-only default.

Text-only and unknown models use the existing `Agent.VisionModel`, OCR, and MCP
vision fallback paths. Raw image payloads are never sent to those models.
