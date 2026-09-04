# 模型能力元数据

Reasonix 通过 Provider Adapter 按具体模型解析输入能力。Adapter 遵循
`deepseek-harness` 的模型契约，为精确模型返回 `inputModalities`：

- `text` 表示支持文本输入；
- `image` 表示支持原生图片输入；
- `text + image` 会在不配置 `VisionModels` 的情况下自动启用多模态请求。

OpenAI-compatible `/models` 响应优先使用标准字段 `input_modalities`，同时
兼容 `modalities.input`、`capabilities.input_modalities`、
`capabilities.vision`、`supports_vision` 和 `vision`。没有正向声明时，通用
Adapter 会将精确模型安全地视为文本模型，不会根据模型名称猜测视觉能力。

动态元数据保存在 Reasonix 缓存目录下独立的
`model-capabilities-v1.json` 文件中，不会写入 `config.toml`。已有的
`vision` 和 `vision_models` 配置仍可读取，并优先于动态元数据。

文本模型和未知模型继续使用现有的 `Agent.VisionModel`、OCR、MCP vision
fallback。原始图片不会发送给这些模型。
