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

内置 Adapter 还提供官方 OpenCode Go 路由、DeepSeek vision SKU 以及精选
ModelScope Qwen3.5 SKU 的本地校验目录，因此不依赖模型列表请求即可工作。
自定义 Endpoint 或本地目录之外的模型仍使用安全的文本模型默认值。

更完整的 Provider/模型目录来源于 MIT 许可的
`github.com/sky-valley/pi/ai` Go 版 Pi。Reasonix 只使用其嵌入的模型数据
（`GetModels`、`Model.Input` 及相关字段），不引入它的 Agent 或 Provider
运行时。依赖版本固定在 `go.mod` 中，目录更新需要按数据和许可证变更审查。

文本模型和未知模型继续使用现有的 `Agent.VisionModel`、OCR、MCP vision
fallback。原始图片不会发送给这些模型。
