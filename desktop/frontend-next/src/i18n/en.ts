// English catalogue, keyed by the Chinese source text. A key that is missing
// here renders as Chinese rather than as a blank, so an incomplete translation
// degrades instead of breaking — and i18n.test.ts is what stops it from
// staying incomplete quietly.
//
// Grouped the way the interface is, not alphabetically: a translator reads a
// screen at a time, and a term is only right relative to the ones beside it.
export const EN: Record<string, string> = {
  // ── 转录：卡片与流 ───────────────────────────────────────────────
  "交待一件事，它自己往下做": "Give it one thing to do, and it runs with it",
  "读代码、联网查证、派子代理、改文件 —— 每一步都同时落进「轨迹」，那是机器记录，不是给人读的叙事。":
    "Reading code, checking the web, delegating, editing files — every step also lands in the Trajectory, which is a machine record, not a story written for you.",
  "把这个仓库跑一遍测试，把失败的那几个定位到具体文件":
    "Run this repository's tests and pin the failures down to specific files",
  "读一遍最近三次提交，告诉我哪里的改动风险最高":
    "Read the last three commits and tell me which changes carry the most risk",
  "查一下这个项目的缓存命中率为什么会掉":
    "Find out why this project's cache hit rate is dropping",
  "等待回包 {secs}s": "Waiting for a response · {secs}s",
  "连接在响应头前断了，重试 {attempt}/{max} · {secs}s":
    "Connection dropped before the headers · retry {attempt}/{max} · {secs}s",
  "Agent": "Agent",
  "思考中…": "Thinking…",
  "想了 {chars}": "Thought · {chars}",
  "想了 {secs} 秒 · {chars}": "Thought for {secs}s · {chars}",
  "{n} 字": "{n} chars",

  // ── 运行状态 ─────────────────────────────────────────────────────
  "空闲": "Idle",
  "运行中": "Running",
  "思考中": "Thinking",
  "正在回答": "Answering",
  "已完成": "Done",
  "等你批准": "Waiting for approval",
  "等你决定": "Waiting on you",
  "缓存": "Cache",
  "{n} 步 · {clock}": "{n} steps · {clock}",
  "{n} 秒": "{n}s",
  "{m} 分 {s} 秒": "{m}m {s}s",

  // ── 面板与栏 ─────────────────────────────────────────────────────
  "活动": "Activity",
  "轨迹": "Trajectory",
  "度量": "Metrics",
  "工作区": "Workspaces",
  "收起工作区栏": "Collapse the workspace rail",
  "收起度量栏": "Collapse the metrics rail",
  "从列表移除": "Remove from the list",
  "改为输入路径": "Type a path instead",
  "输入文件夹路径": "Type the folder's path",
  "界面大小": "Interface size",
  "正文字号": "Body text size",
  "子代理": "Subagents",
  "尚未派出": "None dispatched",
  "{n} 并行": "{n} in parallel",
  "已交活": "Handed back",
  "待审改动": "Pending changes",
  "改动 · 已放行": "Changes · auto-approved",
  "尚无改动": "No changes yet",
  "更早的 {n} 个未列出": "{n} earlier files not listed",
  "上下文": "Context",
  "上下文构成": "What fills the context",
  "系统提示": "System prompt",
  "基础指令、记忆、技能清单": "Base instructions, memory, the skill list",
  "工具定义": "Tool definitions",
  "发给模型的工具清单": "The tool list sent to the model",
  "你说的话": "What you said",
  "这一会话里你输入的部分": "Your side of this session",
  "模型回复": "Model replies",
  "模型说过的话": "What the model has said",
  "工具输出": "Tool output",
  "命令、读取、检索返回的内容": "What commands, reads and searches returned",
  "估算值，和触发压缩用的是同一把尺子": "An estimate, measured the same way compaction is triggered",
  "前缀命中": "Prefix hits",
  "命中": "Hit",
  "未命中": "Missed",
  "输出": "Output",
  "下行": "Down",
  "命中 {rate}%": "{rate}% hit",
  "若不命中": "If it all missed",
  "价目未上报": "No pricing reported",
  "主循环": "Main loop",
  "压缩": "Compaction",
  "规划": "Planning",
  "分类": "Classifying",
  "标题": "Titling",
  "本会话": "This session",
  "计划": "Plan",
  "尚未制定": "None yet",
  "后台任务": "Background jobs",
  "无": "None",
  "外部服务": "External services",
  "{n} 个连不上": "{n} unreachable",
  "到设置里重连": "Reconnect from Settings",
  "失败": "Failed",
  "OpenAI 兼容": "OpenAI-compatible",
  "Anthropic 兼容": "Anthropic-compatible",
  "扩展": "Extension",
  "新增": "Added",
  "已删除": "Deleted",
  "重命名": "Renamed",

  // ── 会话树 ───────────────────────────────────────────────────────
  "新会话": "New session",
  "从左边挑一个，或者在当前文件夹开一个新的": "Pick one on the left, or start a new one in this folder",
  "开一个新会话": "Start a new session",
  "空会话": "Empty session",
  "{n} 轮": "{n} turns",
  "还有 {n} 个 · 全部显示": "{n} more · show all",
  "添加文件夹…": "Add a folder…",
  "文件夹的完整路径": "Full path to the folder",
  "还没有文件夹 —— 从下面添加一个": "No folders yet — add one below",
  "隔离": "Isolated",
  "移除": "Remove",
  "删除": "Delete",
  "从列表移除（不删除任何文件）": "Remove from the list (deletes no files)",
  "删除这个会话": "Delete this session",
  "重命名这个会话": "Rename this session",
  "展开": "Expand",
  "收起": "Collapse",
  "在 {name} 下开一个新会话": "Start a new session in {name}",
  "最多同时开 {n} 个面板，先关掉一个": "At most {n} panes at once — close one first",
  "关闭这个会话面板": "Close this pane",
  "这个文件夹还有打开的面板，先关掉再移除": "This folder still has open panes — close them before removing it",

  // ── 输入区 ───────────────────────────────────────────────────────
  "↓ 回到最新": "↓ Back to latest",
  "停下": "Stop",
  "发送": "Send",
  "交待一个任务，回车发送…　/ 调用命令与技能，@ 引用文件":
    "Describe a task, press Enter…　/ for commands and skills, @ to reference files",
  "知道了": "Got it",
  "强度": "Effort",
  "批准": "Approvals",
  "移除这张图": "Remove this image",
  "添加图片": "Add an image",
  "添加图片　也可以直接拖进来或粘贴": "Add an image　or just drop or paste one in",
  "当前模型不读图 · 将交给能读图的子代理": "This model does not read images · a subagent that does will take it",
  "询问": "Ask",
  "每次动手前问你。": "Asks before every action.",
  "自动": "Auto",
  "低风险自己过，写操作仍然问。": "Low-risk steps proceed; writes still ask.",
  "不再问": "Stop asking",
  "这一类记住，本会话不再问。": "Remembers this kind for the rest of the session.",
  "全放行": "Allow all",
  "不问了。只在你完全信任这个工作区时用。": "No more questions. Only for a workspace you fully trust.",
  "均衡": "Balanced",
  "交付": "Delivery",
  "执行设定": "Execution",
  "插话待送达": "Queued",
  "登录": "Sign in",
  "登录（社区与崩溃跟进，不影响使用）": "Sign in (community and crash follow-up; not required to use it)",
  "账号：{name}": "Account: {name}",

  // ── 外观设置 ─────────────────────────────────────────────────────
  "外观": "Appearance",
  "大小": "Size",
  "界面": "Interface",
  "正文": "Body text",
  "紧凑": "Compact",
  "标准": "Standard",
  "宽松": "Comfortable",
  "更大": "Larger",
  "小": "Small",
  "大": "Large",
  "「界面」连边距和控件一起缩放，「正文」只动对话里的字。两个各调各的。":
    "“Interface” scales spacing and controls along with the type; “Body text” moves only the words in the conversation. They are set independently.",
  "字体": "Typeface",
  "等宽": "Monospace",
  "清除": "Clear",
  "回到默认字体": "Back to the default typeface",
  "默认": "Default",
  "默认 · 本机有 {n} 个可选": "Default · {n} available on this machine",
  "输入框里可以直接写字体名，也可以从这台机器装了的里面挑。下面那行就用它画 —— 没变样说明这个名字在这台机器上找不到，界面会退回默认字体，不会弄花。":
    "Type any family name, or pick one of the ones installed here. The line below is drawn in it — if nothing changes, that name is not on this machine, and the interface falls back to its default rather than breaking.",
  "交待一件事，它自己往下做 · Aa Bb 0123": "The quick brown fox jumps over it · Aa Bb 0123",
  "壁纸": "Wallpaper",
  "选一张图片…": "Choose an image…",
  "换一张…": "Choose another…",
  "浓度": "Strength",
  "压暗": "Dim",
  "焦点": "Focus",
  "横向焦点": "Horizontal focus",
  "纵向焦点": "Vertical focus",
  "图片只铺在窗口的空白处，卡片和输入框始终不透明 —— 背景值一圈留白，不值一段读不清的正文。跑起来的时候它会自动退到更淡。":
    "The picture fills only the window's empty margins; cards and the composer stay opaque — a background is worth a margin, not an unreadable paragraph. It recedes further while a turn is running.",
  "这种图片格式用不了，换 PNG、JPEG、WebP、AVIF 或 GIF":
    "That image format will not work — use PNG, JPEG, WebP, AVIF or GIF",
  "图片是空的": "The image is empty",
  "有活儿在跑，先停下再换模型": "Something is running — stop it before switching models",
  "有活儿在跑，先停下再改推理强度": "Something is running — stop it before changing the reasoning effort",
  "有活儿在跑，先停下再换工作区": "Something is running — stop it before changing the workspace",
  "有活儿在跑，先停下再重载扩展": "Something is running — stop it before reloading extensions",
  "这个来源正在用，先换一个模型再删": "This source is in use — switch models before deleting it",
  "图片太大了，先压到 {n} MB 以内": "That image is too large — bring it under {n} MB first",
  "语言": "Language",
  "跟随系统": "Follow the system",
  "中文": "中文",
  "英文": "English",
  "界面用哪种语言。它跟模型回你话的语言是两件事 —— 模型跟着你这条消息用的语言走。":
    "Which language the interface is in. This is separate from the language the model answers in — that follows whatever you wrote, per message.",
  "改语言要重开窗口才生效": "Changing the language reloads the window",

  // ── 明暗与配色 ───────────────────────────────────────────────────
  "明暗": "Light and dark",
  "浅色": "Light",
  "深色": "Dark",
  "配色": "Palette",
  "文字对比度": "Text contrast",
  "柔和": "Soft",
  "更强": "Stronger",
  "内置": "Built in",
  "第三方": "Third party",
  "{n} 个已装": "{n} installed",
  "跟随系统时，系统切换会立刻反映；手动选过就固定住。":
    "Following the system reflects its changes immediately; an explicit choice stays put.",

  // ── 设置：分区与导航 ─────────────────────────────────────────────
  "设置分类": "Settings sections",
  "这个设置分区出错了": "This settings section failed",
  "其它分区和你的会话不受影响；关掉设置再打开可重试。":
    "Other sections and your session are unaffected; close Settings and reopen to try again.",
  "这一步没做成": "That did not go through",

  // ── 设置：会话 ───────────────────────────────────────────────────
  "管的是「做完了」谁说了算。切档立刻生效，不重建运行时。":
    "This decides who gets to say a thing is done. Switching takes effect at once and rebuilds nothing.",
  "计划模式": "Plan mode",
  "开着的时候拿不到写权限：这不是提示词里的约定，是没给这个能力。":
    "While this is on, writing is not available — not as a promise in a prompt, but as a capability that was never handed over.",
  "开": "On",
  "关": "Off",
  "只读加出计划，你批准后核心自己关掉它":
    "Reads and drafts a plan; once you approve it the kernel turns this off itself",
  "正常执行": "Runs normally",
  "这个会话在哪写": "Where this session writes",
  "工作目录": "Working directory",
  "文件夹在左栏管：底部添加，展开后开新会话。一个会话属于开它的那个文件夹，不会跟着跑到别处。":
    "Folders are managed in the left rail: add one at the bottom, expand it to start a session. A session belongs to the folder it was opened in and does not follow you elsewhere.",
  "拉一份隔离副本": "Work on an isolated copy",
  "在 Git worktree 里开一份，改动不落回当前分支":
    "Opens a git worktree; edits do not land on the current branch",

  // ── 设置：模型 ───────────────────────────────────────────────────
  "模型": "Model",
  "分工": "Roles",
  "每个位置默认跟着主模型走，只有你明确指派过的才会分出去。换指派跟换主模型一样要重建运行时，有活儿在跑的时候换不了。":
    "Every role follows the main model unless you assign it somewhere else. Reassigning rebuilds the runtime the way switching the main model does, so it cannot happen while work is running.",
  "切换会带着对话重建运行时；有活儿在跑的时候切不了。标签只写探得到的能力 —— 空着就是没人声明过，不是「不支持」。":
    "Switching rebuilds the runtime around the conversation, so it cannot happen mid-run. The tags list only capabilities the endpoint actually reports — blank means nobody declared one, not that it is unsupported.",
  "推理强度": "Reasoning effort",
  "这几档是当前模型的端点真正认的，auto 表示交给它自己的默认。":
    "These are the levels this model's endpoint really accepts; auto leaves it to the endpoint's own default.",
  "当前模型没有暴露可调的推理档位，调它不会有任何效果，所以这里不给开关。":
    "This model exposes no adjustable reasoning level. Setting one would do nothing, so there is no control here.",
  "连接": "Connections",
  "模型从哪里来。添加只问地址和 key —— 协议、模型列表、能不能看图，都去问端点，问不出来的才让你填。":
    "Where models come from. Adding one asks only for an address and a key — the protocol, the model list and whether it reads images are asked of the endpoint, and you are only asked for what it will not answer.",

  // ── 设置：工具与权限 ─────────────────────────────────────────────
  "工具批准": "Tool approvals",
  "这是唯一挡在 agent 和你的文件之间的闸。它拦下来的时候，没有第二个入口能绕过去。":
    "This is the only gate between the agent and your files. When it stops something, there is no second door around it.",
  "明确的规矩": "Rules you spell out",
  "上面那档管的是「问不问你」，这里管的是「哪些根本不许，哪些永远不用问」。改动会重建运行时，有活儿在跑的时候改不了。":
    "The setting above decides whether you are asked. These decide what is never allowed and what never needs asking. A change rebuilds the runtime, so it cannot happen while work is running.",
  "读不到权限配置。": "Cannot read the permission config.",
  "这个项目自带一份权限配置": "This project ships its own permissions",
  "{path} 里也写了 permissions，实际生效的是它。这里的改动会存下来，但要等它不再声明才用得上。":
    "{path} declares permissions too, and that is what is in force. Edits here are saved, but only take effect once it stops declaring them.",
  "不许动 .env": "Keep .env off limits",
  "文件工具读或写 .env 一律拒绝。bash 里的 cat 走的是另一条路，这条管不到它":
    "File tools are refused outright on .env. A cat inside bash goes through another path this rule cannot reach",
  "不许推到远端": "Never push",
  "本地怎么改都行，推出去这一步永远留给你": "Change anything locally; pushing stays yours alone",
  "不许改写 git 历史": "Never rewrite git history",
  "rebase 和 reset 能吃掉你还没推的提交，这两个命令直接拒绝":
    "rebase and reset can eat commits you have not pushed, so both are refused",
  "跑测试不用问": "Tests without asking",
  "测试命令直接放行；其余命令照旧": "Test commands go straight through; everything else is unchanged",
  "剩下的写操作": "Everything else that writes",
  "问我": "Ask me",
  "没被上面三组说中的写操作，动手前问一次": "A write no list matched asks once before it happens",
  "放行": "Allow",
  "没被说中的写操作直接做": "A write no list matched just happens",
  "拒绝": "Refuse",
  "没被说中的写操作一律不做": "A write no list matched never happens",
  "一律拒绝": "Always refuse",
  "先看这一组。命中了就没有商量的余地：全放行也拦得住，批准框不会出现。":
    "Checked first. A match is final: it holds even under full access, and no approval box appears.",
  "每次都问": "Always ask",
  "命中的调用一定停下来等你，哪怕自动批准开着。": "A match always stops for you, even with auto-approval on.",
  "直接放行": "Always allow",
  "命中的调用不再问你。没命中的按下面那档处理。": "A match stops asking. Anything else falls through to the setting below.",
  "写法：工具名，或者工具名(要匹配的东西)。bash 按命令的词来比，所以 bash(git push:*) 挡得住带任何参数的 git push；文件工具比的是路径，* 能跨过 /。file_mutation 代表所有会写文件的工具。":
    "Format: a tool name, or tool name(what it matches). bash compares command words, so bash(git push:*) catches git push with any arguments; file tools compare paths, where * crosses /. file_mutation stands for every tool that writes a file.",
  "删掉 {rule}": "Delete {rule}",
  "来自项目配置": "From project config",
  "加上": "Add",
  "例如 bash(go build:*)": "e.g. bash(go build:*)",
  "例如 bash(rm:*)": "e.g. bash(rm:*)",

  "沙箱": "Sandbox",
  "批准之后能碰到多大范围。这一层不靠 agent 自觉：写入范围由工具执行，命令隔离由操作系统执行。":
    "How far an approved call reaches. This layer does not rely on the agent behaving: the write range is enforced by the tools, the command jail by the operating system.",
  "读不到沙箱配置。": "Cannot read the sandbox config.",
  "这个项目自带一份沙箱配置": "This project ships its own sandbox",
  "{path} 里也写了 sandbox，实际生效的是它。": "{path} declares a sandbox too, and that is what is in force.",
  "能写到哪": "Where writes may land",
  "批准过的写操作也只能落在这些目录里。这不是提示词里的约定，是文件工具拿不到别处的句柄。":
    "Even an approved write can only land in these directories. This is not a promise in a prompt — the file tools have no handle to anywhere else.",
  "工作区根目录": "Workspace root",
  "留空 = 会话所在的目录": "Empty = the session's own directory",
  "不再允许写 {path}": "Stop allowing writes to {path}",
  "再开一个可写目录，例如 /tmp/scratch": "Open one more writable directory, e.g. /tmp/scratch",
  "另外还能写": "Also writable",
  "实际可写": "Actually writable",
  "命令怎么跑": "How commands run",
  "关进沙箱之后，命令连想写别处都做不到 —— 上面那份可写清单会由操作系统来执行，而不是由 agent 自觉遵守。":
    "Jailed, a command cannot write elsewhere even if it tries — the list above is enforced by the operating system rather than observed by the agent.",
  "这台机器没有可用的 OS 沙箱": "This machine has no usable OS sandbox",
  "命令只能不受限地运行；上面的可写范围仍然由工具自己执行。":
    "Commands can only run unconfined; the write range above is still enforced by the tools themselves.",
  "关进沙箱": "Jailed",
  "不受限": "Unconfined",
  "沙箱里允许联网": "Allow network inside the sandbox",
  "关掉之后装依赖、拉仓库都会失败 —— 这正是它的用途":
    "With this off, installing dependencies and cloning repos fail — which is the point",

  "宣告做完": "Claims it is done",
  "还在做": "Still working",
  "卡住了，要你介入": "Stuck — needs you",
  "端点要求的额外设置": "Extra settings this endpoint demands",
  "上下文窗口（tokens）": "Context window (tokens)",
  "留空 = 用内置的已知值；0 = 这个来源不自动压缩":
    "Empty = use the built-in known value; 0 = never auto-compact on this source",
  "填模型文档写的上下文上限，不是最大输出。填小了会一直压缩，填大了会在真到上限时被端点拒绝。":
    "Use the context limit from the model's documentation, not its max output. Too small and it compacts constantly; too large and the endpoint refuses once the real limit is hit.",
  "额外请求头": "Extra request headers",
  "一行一个 名字: 值。中转站常要它来认站点；密钥仍然走上面那栏。":
    "One name: value per line. Relays often require these to identify the site; the key still goes in the field above.",
  "额外请求体": "Extra request body",
  "会并进请求体的顶层。model、messages、tools、stream 这些仍由内核说了算，写了也不生效。":
    "Merged into the top level of the request body. model, messages, tools and stream stay the kernel's to set, so writing them here has no effect.",
  "这段不是合法的 JSON 对象，保存会被拒绝。": "This is not a valid JSON object, and saving would be refused.",
  "不压缩": "no compaction",
  "{n} 个头": "{n} headers",
  "有请求体": "extra body",
  "命令交给谁执行": "Which program runs commands",
  "agent 的每条命令都由这个程序来跑，所以它也决定命令该写成哪一种语法 —— 选错了不是慢，是每条都报错。下面列的是这台机器上真有的，装什么才能选什么。换一个要重建运行时，有活儿在跑的时候换不了。":
    "Every command the agent runs goes through this program, so it also decides which syntax those commands must be written in — the wrong one is not slower, it fails every time. Listed below is what is really installed here. Changing it rebuilds the runtime, so it cannot happen while work is running.",

  // ── 设置：自动化与扩展 ───────────────────────────────────────────
  "自动化": "Automation",
  "在 agent 干活的前后插进你自己的命令。它们跑在你的机器上，用你的权限 —— 挡得住 agent 的那两个事件在下面会标出来。":
    "Run your own commands before and after the agent acts. They run on your machine with your permissions — the two events that can actually stop the agent are marked below.",
  "插件包": "Plugin packages",
  "一个包能一次带来技能、命令、自动化钩子和外部服务。装和导入是同一件事：给它一个仓库地址，或者机器上的一个文件夹。":
    "One package can bring skills, commands, hooks and external services at once. Installing and importing are the same act: give it a repository address, or a folder on this machine.",
  "还没装插件包。": "No plugin packages installed.",
  "外部工具": "External tools",
  "你自己接进来的 MCP 服务。它给 agent 的能力和内置工具一样真实 —— 列在这里的每一项都能动你的东西。关掉一个会立刻从这一轮的工具表里消失，并且重启后依然是关的。":
    "MCP services you connected yourself. What they give the agent is as real as any built-in tool — everything listed here can touch your things. Switching one off drops it from this turn's tool list immediately, and it stays off across restarts.",
  "接入服务": "Connect a service",
  "没有自己接入的外部服务。": "No external services connected.",
  "技能": "Skills",
  "这个工作目录下没有技能。": "No skills in this working directory.",
  "没有写说明": "No description written",
  "只读": "Read-only",

  // ── 设置：其余分区 ───────────────────────────────────────────────
  "网络": "Network",
  "模型请求、MCP 的远程服务、网页抓取都走这里。配错了通常表现为聊天时莫名其妙卡住 —— 所以先测一下，它会告诉你断在哪一段。":
    "Model requests, remote MCP services and web fetches all go through here. Getting it wrong usually shows up as a chat that inexplicably hangs — so run the test first; it tells you which hop broke.",
  "账号": "Account",
  "Reasonix 本身不需要账号。它只用在天生要联网的地方：社区发帖、崩溃问题跟进，以后还有技能发布。":
    "Reasonix itself needs no account. One is only used where the network is inherent: posting to the community, following up a crash, and publishing skills later on.",
  "已登录": "Signed in",
  "未登录": "Not signed in",
  "版本": "Version",
  "装的是哪一版、有没有更新，以及出问题时怎么退回去。回退会固定在你选的版本，不会被自动更新拽回来。":
    "Which version is installed, whether there is an update, and how to go back when something breaks. A rollback pins the version you chose; automatic updates will not drag you forward again.",
  "记忆": "Memory",
  "它自己记下来的东西 —— 你没配置过，但它会照着做。所以这里按「什么时候会被想起」分，并且标出上一轮真正用上了哪几条。":
    "What it wrote down for itself — nothing you configured, but it acts on all of it. So these are grouped by when they get recalled, and the ones the last turn actually used are marked.",
  "还不在这一版里": "Not in this version yet",
  "每一项都需要自己的界面，做半个不如先说清楚它现在在哪。":
    "Each of these needs an interface of its own. Half of one is worth less than saying plainly where it stands today.",
  "旧版桌面端": "Previous desktop app",

  // ── 设置：行内操作 ───────────────────────────────────────────────
  "添加": "Add",
  "算了": "Never mind",
  "配置": "the config",
  "把 {name} 从 {where} 里删掉？只是想暂时不用的话，关掉开关就够了。":
    "Delete {name} from {where}? If you only want it out of the way for now, the switch is enough.",
  "移除 {name}": "Remove {name}",
  "关闭 {name}": "Disable {name}",
  "启用 {name}": "Enable {name}",
  "同名的另一处声明现在生效了，这一行不会消失。":
    "Another declaration of the same name took over, so this row will not disappear.",
  "{n} 条": "{n}",
  "{n} 个包": "{n} packages",
  "{n} 个异常": "{n} with problems",
  "{n} 个工具": "{n} tools",

  // ── 连接与来源 ───────────────────────────────────────────────────
  "添加模型来源": "Add a model source",
  "还没有配置任何模型来源。": "No model sources configured yet.",
  "正在读取…": "Loading…",
  "名字": "Name",
  "接口地址": "Endpoint address",
  "API Key（留空就不动它）": "API key (leave blank to keep the current one)",
  "接入方式": "Protocol",
  "联网搜索": "Web search",
  "端点自己执行的搜索，不占本地工具。": "The endpoint runs the search itself; it costs no local tool.",
  "思考参数": "Thinking parameter",
  "读图": "Reads images",
  "推理": "Reasoning",
  "自己写一条": "Write one myself",
  "连不上": "Unreachable",
  "连上了": "Connected",
  "{n} 条规则": "{n} rules",
  "忘掉": "Forget",
  "重连": "Reconnect",
  "重新授权": "Re-authorise",
  "记的是 {had}，但它答的是 {got}。": "Recorded as {had}, but it answers as {got}.",
  "同一个账号的两扇门。这一扇没有联网搜索 —— 那是协议的差别，不是设置。":
    "Two doors onto one account. This one has no web search — that is a difference between protocols, not a setting.",
  "同一个账号的两扇门。换一扇，下面的模型跟着换。":
    "Two doors onto one account. Switch doors and the models below follow.",
  "正在用": "In use",
  "缺 key": "no key",
  "编辑": "Edit",
  "测试中…": "Testing…",
  "测一下": "Test it",
  "没保存成功": "Not saved",
  "重新问一次有哪些模型": "Ask for the model list again",
  "重新问这个端点要一次模型列表 —— 它上新或下架模型之后用":
    "Ask this endpoint for its model list again — for after it adds or retires models",
  "探到了下面这些。都是猜的，不对就改。": "Here is what was probed. All of it is a guess; correct anything wrong.",
  "两种接入方式的模型列表它都答得上来，光看列表分不出来 —— 聊天入口通常不在同一个路径下，选错了聊天会报错。要两条都用就再添加一次、选另一个。":
    "It answers the model list on both protocols, so the list alone cannot tell them apart — the chat endpoint usually lives at a different path, and the wrong choice fails at chat time. To use both, add it a second time and pick the other one.",
  "就是给它再开一扇门，两条会并成同一个来源、由「接入方式」切换；填了 key就是这台机器上的另一个账号，各算各的。":
    "leaves it as a second door onto the same source, switched by Protocol; giving a key makes it a separate account on this machine, counted separately.",
  "走代理连不上、直连可以，已经记成「这个来源不走代理」。":
    "The proxy could not reach it and a direct connection could, so this source is now recorded as bypassing the proxy.",
  "{n} 个模型": "{n} models",

  // ── 首次连接 ─────────────────────────────────────────────────────
  "先连一个模型服务": "Connect a model service first",
  "填地址和 key，剩下的问它自己 —— 协议、模型清单、能不能读图，都是探得到的。":
    "Give it an address and a key; the rest is asked of the endpoint — protocol, model list, whether it reads images.",
  "key 存进本机配置，不上传任何第三方。模型、推理强度、执行设定都有默认值，随时能在输入框那排改。":
    "The key goes into this machine's config and is uploaded nowhere. Model, reasoning effort and execution settings all have defaults you can change from the composer at any time.",
  "常用服务": "Common services",
  "服务地址": "Service address",
  "https://你的地址/v1": "https://your-host/v1",
  "先用哪个": "Which one to start with",
  "选择默认模型": "Choose the default model",
  "这个端点不止一种协议答应了，上面是偏好而不是事实。连上之后能在设置里换。":
    "More than one protocol answered at this endpoint, so the choice above is a preference rather than a fact. You can change it in Settings once connected.",

  // ── 外部服务与插件 ───────────────────────────────────────────────
  "一段 JSON、一行命令，或者一个 https 地址": "A block of JSON, a command line, or an https address",
  "装到哪": "Where to install it",
  "所有项目里都能用": "Available in every project",
  "写进仓库，clone 的人也会拿到": "Written into the repository, so whoever clones it gets it too",
  "返回": "Back",
  "完成": "Done",
  "一个仓库地址，或者把文件夹拖进来": "A repository address, or drop a folder in",
  "选文件夹": "Choose a folder",
  "会加进来": "Will be added",
  "要你填": "Needs filling in",
  "用不了": "Unusable",
  "更新": "Update",
  "常驻进程": "Long-running process",

  // ── 自动化 ───────────────────────────────────────────────────────
  "加一条": "Add one",
  "删掉": "Delete",
  "要运行的命令": "The command to run",
  "匹配哪些工具（正则，留空=全部）": "Which tools to match (regex; blank means all)",
  "会真的执行这条命令": "Really runs this command",
  "能挡住 agent": "Can stop the agent",
  "这些规则写在哪": "Where these rules live",
  "读不到 hooks 配置。": "Could not read the hooks config.",

  // ── 网络 ─────────────────────────────────────────────────────────
  "代理模式": "Proxy mode",
  "始终直连": "Always connect directly",
  "协议": "Protocol",
  "服务器": "Server",
  "端口": "Port",
  "用户名（可选）": "Username (optional)",
  "密码（可选）": "Password (optional)",
  "删掉已保存的密码": "Delete the saved password",
  "这些地址直连": "Reach these addresses directly",
  "测试目标": "Test target",
  "读不到网络配置。": "Could not read the network config.",

  // ── 命令解释器 ───────────────────────────────────────────────────
  "当前生效": "In effect",
  "指定一个可执行文件": "Point at an executable",
  "取消指定": "Clear it",
  "可执行文件路径": "Path to the executable",
  "自己编的 bash、MSYS2、装在别处的 pwsh 都填这里。保存前会真的拿它跑一条命令，跑不起来就不会写进配置。":
    "A bash you built yourself, MSYS2, a pwsh installed elsewhere — all go here. Before saving, one command is really run through it; if that fails, nothing is written to the config.",
  "没换成": "Not changed",
  "够不着这个工作目录。": "cannot reach this working directory.",
  "这台机器上没有 bash，所以命令只能按 PowerShell 写。装一个 Git for Windows就会多出 Git Bash 这一项 —— WSL 里的那个不算，它看到的是 /mnt 下的另一套路径，够不着这个工作目录。":
    "There is no bash on this machine, so commands can only be written for PowerShell. Installing Git for Windows adds Git Bash here — the one inside WSL does not count: it sees a different set of paths under /mnt and cannot reach this working directory.",
  "读不到 shell 配置。": "Could not read the shell config.",

  // ── 卡片：批准、还原、收工 ───────────────────────────────────────
  "要动手了": "About to act",
  "允许这一次。": "Allowed once.",
  "本会话不再问这一类。": "Will not ask again this session for this kind.",
  "已拒绝。": "Denied.",
  "推荐": "Recommended",
  "其他 —— 我自己写": "Something else — I will write it",
  "你": "You",
  "我": "Me",
  "排队中 · 下一个工具边界送达": "Queued · delivered at the next tool boundary",
  "改动": "Changes",
  "独立上下文 · 不进主轨迹": "Its own context · stays out of the main trajectory",
  "计划已进右栏": "The plan is in the right rail",
  "收工检查": "Wrap-up check",
  "看它接着往下用的简报": "See the digest it carries forward",
  "正在还原…": "Restoring…",
  "撤销这次还原": "Undo this restore",
  "仍然还原剩下的": "Restore the rest anyway",
  "没能还原": "Could not restore",
  "这一轮没有改动文件": "This turn changed no files",
  "这一轮有改动还原不了": "Some of this turn's changes cannot be restored",

  // ── 开场 ─────────────────────────────────────────────────────────
  "我是小 R。": "I am R.",
  "你交待一件事，我把它做完。": "You give it one thing, and I see it through.",
  "读代码、查证、动手、验证 —— 不是给你一段建议就完了。":
    "Reading code, checking facts, doing the work, verifying it — not handing you advice and stopping there.",
  "每一步都留得下来。": "Every step stays on the record.",
  "改了哪个文件、跑了哪条命令、花了多少 token，都能翻回去看。":
    "Which file changed, which command ran, how many tokens it cost — all of it can be read back.",
  "按任意键跳过": "Press any key to skip",

  // ── 版本 ─────────────────────────────────────────────────────────
  "正在读取版本…": "Loading versions…",
  "连不上版本目录": "Cannot reach the version index",
  "已固定": "Pinned",
  "固定在这里": "Pin here",
  "清除固定": "Clear the pin",
  "恢复自动更新": "Resume automatic updates",
  "在下面那一行装它，安装完会自动重启。": "Install it from the row below; it restarts itself when done.",
  "回退之后固定是有意的：否则下次更新会把你放回刚离开的那个版本。":
    "Pinning after a rollback is deliberate: otherwise the next update would put you back on the version you just left.",
  "这条固定已经不再描述现实，自动更新按未固定处理。":
    "This pin no longer describes anything real, so automatic updates treat it as unpinned.",
  "切换版本期间请不要关窗口。较新版本写过的会话，回到旧版本后会暂时打不开，升回去就能看。":
    "Do not close the window while versions are switching. A session written by a newer version will not open on an older one until you upgrade again.",

  // ── 账号 ─────────────────────────────────────────────────────────
  "正在检查登录状态…": "Checking sign-in status…",
  "上次的登录已过期。": "The previous sign-in has expired.",
  "只清除本地登录凭证，不影响会话、记忆和配置":
    "Clears the local credential only; sessions, memory and config are untouched",
  "用于社区发帖和崩溃问题跟进。与你的模型 API key 无关 —— 登录不会上传你的对话、代码或密钥。":
    "Used for posting to the community and following up crashes. Unrelated to your model API keys — signing in uploads none of your conversations, code or secrets.",

  // ── 分工与模型 ───────────────────────────────────────────────────
  "对话 · 主模型": "Conversation · main model",
  "跟随主模型": "Follow the main model",
  "读不到分工。": "Could not read the role assignments.",
  "搜索模型": "Search models",
  "搜模型名，或输入「图」只看能读图的…": "Search by name, or type “vision” for the ones that read images…",
  "没有匹配的模型。": "No models match.",
  "读不到模型列表。": "Could not read the model list.",

  // ── 记忆 ─────────────────────────────────────────────────────────
  "还没有记下任何东西。": "Nothing has been written down yet.",
  "一直生效": "Always on",
  "相关时才被想起": "Recalled when relevant",
  "只有这一轮看起来相关时才会被翻出来": "Only surfaced when this turn looks like it needs them",
  "工作目录与「我的」里的技能。带 / 的可以自己点名调用；其余的由模型按任务自行判断要不要用。关掉的那些两条路都走不通。改动在下一次新建会话时进入模型的索引。":
    "Skills from the working directory and from Mine. The ones with a / you can call by name; the rest the model decides about on its own, per task. A skill switched off is reachable by neither route. Changes enter the model's index at the next new session.",
  "模型自动发现已关闭：现在只有你点名的技能会跑。改动在下一次新建会话时生效。":
    "Model discovery is off: only the skills you name by hand will run. Changes take effect at the next new session.",
  "每一轮都在提示词里，等同于你给它的长期指令":
    "In the prompt every turn — the same standing as an instruction you gave it yourself",
  "，用上了 {n} 条": " · {n} of them used",
  "，一条都没用上": " · none of them used",
  "上一轮用上了": "Used last turn",
  "上一轮因为「{why}」被翻出来": "Surfaced last turn because of “{why}”",
  "上一轮从「{q}」出发翻了一次记忆": "Last turn searched memory from “{q}”",
  "只能点名": "By name only",
  "只能模型自选": "Model's choice only",
  "调不到": "Unreachable",
  "改完文件自动格式化": "Format files after they are edited",
  "每次写入之后跑一遍格式化命令，失败了只提醒，不打断":
    "Runs a formatter after every write; a failure only warns and does not interrupt",
  "写密钥文件前问我一声": "Ask me before writing a secrets file",
  "碰 .env 之类的路径时挡下来，由你决定要不要放行":
    "Stops on paths like .env and leaves the call to you",
  "任务做完通知我": "Notify me when a task finishes",
  "一轮结束时响一下，人不用一直盯着": "Chimes at the end of a turn so nobody has to watch it",
  "收工前跑一遍测试": "Run the tests before wrapping up",
  "一轮结束时跑测试，红了会作为提醒显示出来": "Runs the tests at the end of a turn; a red one shows up as a warning",
  "已过期": "Expired",
  "读不到记忆。": "Could not read the memory.",

  // ── 其余 ─────────────────────────────────────────────────────────
  "没有打开的会话": "No session open",
  "没有匹配的项": "Nothing matches",
  "筛选": "Filter",
  "补全": "Completions",
  "交还给插件": "Hand back to the extension",
  "实时事件流 · 仅本次连接，切换或重进会话后重建":
    "Live event stream · this connection only; it is rebuilt when you switch or reopen a session",
  "连不上内核：/status 没有回应。": "Cannot reach the kernel: /status did not answer.",
  "深色底上接近纯白的正文会起光晕，太淡的次要文字又读不清 —— 这一档同时管两头。看着累就往「柔和」调。":
    "Near-white body text glows on a dark ground, and secondary text that is too faint cannot be read — this setting moves both ends at once. If your eyes tire, go toward Soft.",
  "装在记忆目录的 themes/ 下，一个目录一个 theme.json。表面、强调色、圆角与字体跟着走；状态色（成功/警告/失败）不跟，那是含义不是装饰。":
    "Installed under themes/ in the memory directory, one theme.json per folder. Surfaces, accent, corner radii and type follow the pack; the status colours (success, warning, failure) do not — those are meaning, not decoration.",
  "还没装配色。把一个带 theme.json 的目录放进 themes/ 就会出现在这里。":
    "No palettes installed. Drop a folder with a theme.json into themes/ and it appears here.",
  "输入框里可以直接写字体名，也可以从这台机器装了的里面挑。下面那行就用它画 ——没变样说明这个名字在这台机器上找不到，界面会退回默认字体，不会弄花。":
    "Type any family name, or pick one of the ones installed here. The line below is drawn in it — if nothing changes, that name is not on this machine, and the interface falls back to its default rather than breaking.",

  // ── 常量表：档位、分区、状态 ─────────────────────────────────────
  "做到模型认为做完为止。日常用这档": "Runs until the model considers it done. The everyday setting",
  "改了东西就得验证、复核、签收，少一样都不算做完":
    "Anything changed must be verified, reviewed and signed off; short of that it is not done",
  "每次动手前问你": "Asks before every action",
  "低风险自己过，写操作仍然问": "Low-risk steps proceed; writes still ask",
  "这一类记住，本会话内不再问": "Remembers this kind and stops asking for the rest of the session",
  "不问了。只在你完全信任这个工作区时用": "Stops asking. Only for a workspace you fully trust",
  "会话": "Session",
  "工具与权限": "Tools and permissions",
  "高级": "Advanced",
  "项目": "Project",
  "自定义": "Custom",
  "我的": "Mine",
  "环境变量": "Environment variables",
  "手动": "Manual",
  "手动设置": "Set manually",
  "直连": "Direct",
  "已连接": "Connected",
  "连接中": "Connecting",
  "已关闭": "Disabled",
  "未连接": "Not connected",
  "本地构建": "Local build",
  "只在这台机器上": "This machine only",
  "这个项目": "This project",
  "导出": "Export",
  "{n} 个已指派": "{n} assigned",
  "所有分工都跟着它": "Every role follows it",
  "已指派": "Assigned",
  "{name}用哪个模型": "Which model for {name}",
  "主模型看不了的图现在没人读得了 —— 会在发出去之前被丢掉。给「看图」指一个带「读图」标签的模型就能接上。":
    "Images the main model cannot read now have no reader — they are dropped before anything is sent. Assign Vision a model tagged as reading images and they get through.",
  "只读地出计划": "Drafts a plan, read-only",
  "派出去的活": "Work sent out",
  "看图": "Vision",
  "读主模型看不了的图": "Reads images the main model cannot",
  "复核": "Review",
  "独立审这一轮": "Audits this turn independently",
  "认得 && 和 ||，但语法仍然是 PowerShell，不是 bash":
    "Understands && and ||, but the syntax is still PowerShell, not bash",
  "不认 && 和 ||，链式命令得拆成两条": "Does not accept && or ||; chained commands must be split in two",
  "保存这个路径": "Save this path",
  "验证中…": "Verifying…",
  "{n} 个分工跟着它": "{n} roles follow it",
  "自己找，优先真 bash。这台机器上会选到": "Finds one itself, preferring a real bash. On this machine that picks",
  "用系统或环境变量里已经设好的代理": "Uses whatever proxy the system or the environment already sets",
  "{n} 个": "{n}",
  "{n} 个技能": "{n} skills",
  "{n} 个子代理": "{n} subagents",
  "{n} 个提示词": "{n} prompts",
  "{n} 套配色": "{n} palettes",
  "{n} 条钩子": "{n} hooks",
  "{n} 个服务": "{n} services",
  "{on}/{all} 开着": "{on}/{all} on",
  "打包中…": "Packing…",
  "导出好了。": "Exported.",
  "存到 {path}。": "Saved to {path}.",
  "{n} 个命令": "{n} commands",

  // ── 面板标签与关闭确认 ───────────────────────────────────────────
  "会话面板": "Session panes",
  "关闭这个面板？": "Close this pane?",
  "关闭 {n} 个面板？": "Close {n} panes?",
  "{names} 还在跑": "{names} is still running",
  "其中 {n} 个还在跑：{names}": "{n} of them are still running: {names}",
  "，关掉会停下它": " — closing stops it",

  // ── 版本 ─────────────────────────────────────────────────────────
  "今天": "today",
  "昨天": "yesterday",
  "{n} 天前": "{n} days ago",
  "下载中 {got} / {all}": "Downloading {got} / {all}",
  "下载中 {got}": "Downloading {got}",
  "校验签名…": "Verifying the signature…",
  "准备安装…": "Preparing to install…",

  // ── 插件安装 ─────────────────────────────────────────────────────
  "这个来源里没有能装的东西": "There is nothing installable at that source",
  "重装 {name}": "Reinstall {name}",
  "这一版新增了：{list}": "This version adds: {list}",
  "没有新增会执行的东西。": "Nothing new that runs.",
  "更新中…": "Updating…",
  "安装中…": "Installing…",
  "装上": "Install",
  "读不到 {name} 的来源": "Cannot read {name}'s source",
  "正在读取 {name} 的来源…": "Reading {name}'s source…",
  "核心给出的判定（{n}）": "The kernel's assessment ({n})",

  // ── 来源与自动化 ─────────────────────────────────────────────────
  "{name} 的接入方式": "Protocol for {name}",
  "{name} 的联网搜索": "Web search for {name}",
  "{name} 的思考参数": "Thinking parameter for {name}",
  "记的是 {had}，但它答的是 {got}": "Recorded as {had}, but it answers as {got}",
  "没有打开项目": "No project open",
  "读不了：": "could not be read: ",
  "插件带来的 {n} 条": "{n} from plugins",
  "超时了，这一步会被当成失败": "Timed out — this step counts as a failure",

  // ── 卡片 ─────────────────────────────────────────────────────────
  "把工作区和对话退回这条消息之前": "Take the workspace and the conversation back to before this message",
  "↩ 回到这里": "↩ Back to here",
  "按 {name} 这份技能的设定跑的子代理": "A subagent running under the {name} skill's settings",
  "外部服务 {name} 提供的工具": "A tool provided by the external service {name}",
  "由 {name} 渲染": "Drawn by {name}",
  "{name} 做了 {n} 步": "{name} took {n} steps",

  // ── 内核拒绝：来源 ───────────────────────────────────────────────
  "给这个来源起个名字": "Give this source a name",
  "名字只能用字母、数字、点、横线和下划线": "A name may use letters, digits, dot, dash and underscore only",
  "填一个接口地址": "Enter an endpoint address",
  "不认识「{kind}」这种接入方式": "“{kind}” is not a protocol this knows",
  "至少挑一个模型": "Pick at least one model",
  "默认模型「{model}」不在挑中的那几个里": "The default model “{model}” is not among the ones you picked",
  "这个协议不发思考参数，开了也没用": "This protocol never sends thinking parameters — turning it on does nothing",
  "这个协议没有让端点自己搜索的写法": "This protocol has no wire format for a search the endpoint runs itself",
  "这台服务器不让改模型来源": "This server does not allow editing model sources",
  "要填 API key": "An API key is required",
  "这个 key 太长了，八成粘错了东西": "That key is too long — something else was probably pasted",
  "已经连上了，不用再配一次": "Already connected — no need to set it up again",
  "远端配置没做成，稍后再试": "The remote setup did not complete; try again shortly",

  // ── 内核拒绝：会话 ───────────────────────────────────────────────
  "这台服务器关掉了会话切换": "This server has session switching turned off",
  "这个会话正在清理，等一下再打开": "This session is being cleaned up; open it again in a moment",
  "这个会话被别处占着，先把那边关掉": "This session is held elsewhere — close it there first",
  "接管这个会话失败了，重开一次窗口": "Could not take over this session; reopen the window",
  "这个会话还开着面板，先关掉那个": "This session still has a pane open — close it first",
  "这个路径不在任何已知的工作区里": "That path is not inside any known workspace",
  "还没有打开的会话": "No session is open yet",

  // ── 通用动作 ─────────────────────────────────────────────────────
  "取消": "Cancel",
  "保存": "Save",
  "关闭": "Close",
  "确定": "OK",
  "重试": "Retry",
  "设置": "Settings",
  "主题": "Theme",
  "主题：{name}": "Theme: {name}",
  "交待一个任务": "Give it a task",
  "返回工作台": "Back to the workbench",
};
