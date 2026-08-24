// The settings pane's half of the English catalogue. Split off en.ts on size
// alone — one catalogue is still the model, and this file is read as its
// continuation: same keying by Chinese source text, same grouping by screen.
export const EN_SETTINGS: Record<string, string> = {
  // ── 设置：分区与导航 ─────────────────────────────────────────────
  "设置分类": "Settings sections",
  "这个设置分区出错了": "This settings section failed",
  "其它分区和你的会话不受影响；关掉设置再打开可重试。":
    "Other sections and your session are unaffected; close Settings and reopen to try again.",
  "这一步没做成": "That did not go through",

  // ── 设置：会话 ───────────────────────────────────────────────────
  "管的是「做完了」谁说了算。切档立刻生效，不重建运行时。":
    "This decides who gets to say a thing is done. Switching takes effect at once and rebuilds nothing.",
  "正常执行：批准过的写操作直接做": "Normal execution: approved writes just happen",
  "开一份": "Open one",
  "开着…": "Opening…",
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
  "拒绝": "Refuse",
  "命中就没得商量：全放行也拦得住，批准框不会出现": "A match is final: it holds under full access, and no approval box appears",
  "询问": "Ask",
  "命中就停下来等你，哪怕自动批准开着": "A match stops for you, even with auto-approval on",
  "放行": "Allow",
  "命中就不再问你": "A match stops asking",
  "命令": "commands",
  "写文件（全部写入工具）": "writing files (every write tool)",
  "读文件": "reading files",
  "整份写入": "whole-file writes",
  "改动文件": "file edits",
  "抓网页": "fetching web pages",
  "全文搜索": "full-text search",
  "找文件": "finding files",
  "按命令的词比对": "matched by command words",
  "比的是搜索式，不是路径": "matched against the search pattern, not a path",
  "比的是网址": "matched against the URL",
  "正好这一个值": "exactly this one value",
  "按路径匹配，* 能跨过 /": "matched by path, * crosses /",
  "整条命令匹配，不是按词前缀": "matches the whole command, not a word prefix",
  "正好这一条命令，多一个参数就不算": "exactly this command — one more argument and it no longer matches",
  "正好这一个路径": "exactly this one path",
  "这个工具的每一次调用": "every call to this tool",
  "以 {cmd} 开头的命令，带什么参数都算": "commands starting with {cmd}, whatever the arguments",
  "命令整体匹配 {pat}": "commands matching {pat} as a whole",
  "正好是 {cmd} 这条命令": "exactly the command {cmd}",
  "路径匹配 {pat} 的调用（* 能跨过 /）": "calls whose path matches {pat} (* crosses /)",
  "正好是 {path} 这个路径": "exactly the path {path}",
  "{n} 条规矩": "{n} rules",
  "还没有额外的规矩": "No rules of your own yet",
  "筛一下…": "Filter…",
  "整个工具": "the whole tool",
  "{rule} 的处理方式": "What happens on {rule}",
  "没有匹配的规矩。": "No rule matches that.",
  "工具": "Tool",
  "处理方式": "Verdict",
  "加一条": "Add one",
  "例如 git push:*　（留空 = 整个工具）": "e.g. git push:*　(empty = the whole tool)",
  "例如 *.env*　（留空 = 整个工具）": "e.g. *.env*　(empty = the whole tool)",
  "文件工具读或写 .env 一律拒绝。bash 里的 cat 走另一条路，这条管不到它":
    "File tools are refused outright on .env. A cat inside bash goes another way this cannot reach",
  "本地怎么改都行，推出去永远留给你": "Change anything locally; pushing stays yours alone",
  "rebase 和 reset 能吃掉还没推的提交": "rebase and reset can eat commits you have not pushed",
  "测试命令直接放行，其余照旧": "Test commands go straight through; everything else is unchanged",
  "没被任何规矩说中的写操作，动手前问一次": "A write no rule matched asks once before it happens",
  "剩下的写操作": "Everything else that writes",
  "问我": "Ask me",
  "没被上面三组说中的写操作，动手前问一次": "A write no list matched asks once before it happens",
  "没被说中的写操作直接做": "A write no list matched just happens",
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
  "留空 = 用内置的已知值；自己加的来源没有内置值，那就是不压缩":
    "Empty = use the built-in known value; a source you added yourself has none, which means no compaction",
  "填模型文档写的上下文上限，不是最大输出。填小了会一直压缩，填大了会在真到上限时被端点拒绝。":
    "Use the context limit from the model's documentation, not its max output. Too small and it compacts constantly; too large and the endpoint refuses once the real limit is hit.",
  "思考参数": "Thinking parameter",
  "{door} 这扇门上没有 {model}，先在下面挑一个它有的模型。":
    "The {door} door does not carry {model}. Pick a model it has from the list below first.",
  "端点用哪种写法控制思考深度。探不出来 —— 中转站转发的是别人的模型，只有你知道后面是什么。选了才有推理强度可调；选错了请求会被端点拒。":
    "Which shape this endpoint controls thinking depth with. No probe answers it — a relay forwards someone else's models, and only you know what is behind it. Declaring one is what gives this source an effort ladder; declaring the wrong one gets the request refused.",
  "自动 · 按模型和地址推断": "Auto · inferred from the model and address",
  "不发思考参数": "Send no thinking parameter",
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
  "运行时": "Runtime",
  "重载运行时": "Reload runtime",
  "重载中": "Reloading",
  "已生效": "Live",
  "改了扩展的代码，或者装、删、开关了插件包之后，用它让改动生效。当前这一轮不受影响，下一轮开始用新的。":
    "Use this after editing an extension's code, or installing, removing or toggling a package. The turn in flight is untouched; the next one runs the new build.",
  "正在重启常驻进程，重新扫描技能、命令和钩子…": "Restarting resident processes and rescanning skills, commands and hooks…",
  "已生效，下一轮开始用新的扩展": "Live — the next turn runs the new extensions",
  "插件包": "Plugin packages",
  "一个包能一次带来技能、命令、自动化钩子和外部服务。装和导入是同一件事：给它一个仓库地址，或者机器上的一个文件夹。":
    "One package can bring skills, commands, hooks and external services at once. Installing and importing are the same act: give it a repository address, or a folder on this machine.",
  "还没装插件包。": "No plugin packages installed.",
  "外部工具": "External tools",
  "你自己接进来的 MCP 服务。它给 agent 的能力和内置工具一样真实 —— 列在这里的每一项都能动你的东西。关掉一个会立刻从这一轮的工具表里消失，并且重启后依然是关的。":
    "MCP services you connected yourself. What they give the agent is as real as any built-in tool — everything listed here can touch your things. Switching one off drops it from this turn's tool list immediately, and it stays off across restarts.",
  "接入服务": "Connect a service",
  "没有自己接入的外部服务。": "No external services connected.",
  "这个服务没写自我说明。": "This service does not describe itself.",
  "上次连上时的记录": "From the last time it connected",
  "上次连上时的记录 · 声明改过，可能对不上了": "From the last time it connected · the declaration changed since",
  "现在没连着，这是上一次连上时它自己给的答复。":
    "Nothing is connected right now; this is what the service itself answered the last time it was.",
  "会改东西": "Changes things",
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

  // ── 卡片与窗口控件 ───────────────────────────────────────────
  "只在这个项目": "This project only",
  "写进仓库": "Commit to the repo",
  "设置　⌘,": "Settings　⌘,",
  "这一步留在上下文里的估算 token": "Estimated tokens this step leaves in context",
  "关闭这个面板": "Close this pane",
  "窗口": "Window",
  "最小化": "Minimize",
  "工作区与会话": "Workspaces and sessions",
  "允许这一次": "Allow once",
  "这一类不再问": "Stop asking for this kind",
  "核心把它记进会话授权，不落盘。": "The kernel records it for this session only; nothing is written to disk.",
  "agent 收到否决，会另想办法或停手。": "The agent takes the refusal and either finds another way or stops.",
  "下次同样的操作仍会问你。": "The same operation will ask again next time.",
  "你想要的第几种做法，写在这儿": "Which one you want, or write your own here",
  "问题 {n}": "Question {n}",
  "未答": "Unanswered",
  "确认（还有 {n} 个没答）": "Confirm ({n} left)",
  "确认": "Confirm",
  "先不选择，直接回复": "Skip the choice and reply instead",
  "（补写过一次）": " (repaired once)",
  "正在折成简报": "Folding into a digest",
  "这一趟没折叠掉什么": "This pass folded nothing",
  "这段做过的 {n} 处改动，简报都写到了": "All {n} changes made here are in the digest",
  "{kept}/{required} 处改动写进了简报": "{kept}/{required} changes are in the digest",
  "还有 {n} 处只剩下索引地址，要用原文得 recall 取回": "{n} more are index entries only; recall fetches the original",
  "正在把 {n} 条消息折成简报": "Folding {n} messages into a digest",
  "折叠了 {n} 条消息": "Folded {n} messages",

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

  // ── 状态与动作 ───────────────────────────────────────────────
  " · 缺 key": " · no key",
  "安装这个版本": "Install this version",
  "保存中…": "Saving…",
  "不发送": "Don't send",
  "当前版本": "Current version",
  "读取中…": "Reading…",
  "覆盖已装的那一份": "Replace the installed copy",
  "勾「读图」的才会收到图片": "Only the ones marked “reads images” receive them",
  "关掉": "Close",
  "还原": "Restore",
  "回退到这个版本": "Roll back to this version",
  "记住了": "Remembered",
  "接入": "Connect",
  "看看是什么": "See what it is",
  "可多选": "Multiple allowed",
  "可以写 ${PROXY_PASSWORD}": "You can write ${PROXY_PASSWORD}",
  "连接中…": "Connecting…",
  "连一下试试": "Try connecting",
  "（留空就用现有那个来源的 key）": " (leave blank to keep the key from the existing source)",
  "启用": "Enable",
  "确认退出": "Confirm sign-out",
  "试跑一次": "Dry-run once",
  "它的面板会先关掉": "Its panel closes first",
  "退出登录": "Sign out",
  "忘掉中…": "Forgetting…",
  "压缩完成": "Compaction done",
  "移除中…": "Removing…",
  "{err}　—— 本地功能不受影响，稍后再试。": "{err} — nothing local is affected; try again later.",
  "已固定在 {v}，不会自动更新": "Pinned to {v}; updates will not move it",
  "（没有正文）": "(no body)",
  "没有可用的贡献": "Nothing usable inside",
  "删掉 {name}？它带来的技能、命令和服务会一起消失。只是想暂时不用的话，关掉开关就够了。": "Delete {name}? The skills, commands and services it brought go with it. To just set it aside, the switch is enough.",
  "已保存，留空即保持不变": "Saved; leave blank to keep it",
  "已经忘掉": "Forgotten",
  "有的中转站不认 thinking 字段，会整个请求拒掉。真遇上了就切「不发送」。": "Some relays reject the whole request over a thinking field. Switch to “Don't send” if you hit that.",
  "运行中…": "Running…",
  "这个端点不接受图片输入，勾了也不会生效": "This endpoint takes no image input; ticking it changes nothing",
  "正在打开…": "Opening…",
  "正在压缩…": "Compacting…",
  "只发普通聊天参数。模型自己该怎么想还怎么想，只是这边不再指定深度。": "Sends plain chat parameters only. The model still thinks as it will; this side just stops naming a depth.",
  "装到「我的」，所有项目都能用": "Install to “Mine”, available in every project",
  "最大化": "Maximize",

  // ── 对比度与钩子示例 ─────────────────────────────────────────
  "系统开了「增强对比度」就用最强的一档": "Uses the strongest step when the system asks for increased contrast",
  "正文没那么刺眼，长时间看更省力": "Softer body text, easier over a long sitting",
  "介于两者之间": "Between the two",
  "环境光很亮，或需要更清楚的边界": "Bright surroundings, or when edges need to be clearer",

  // 配置文件读不了时顶在设置最上面那条
  "配置文件第 {line} 行读不了": "Line {line} of the config file does not parse",
  "配置文件读不了": "The config file does not parse",
  "下面显示的是上一次能读通的那份设置。这个文件不会被覆盖，所以现在什么都存不进去。": "What is shown below is the last version that read cleanly. The file is never overwritten, so nothing can be saved until it is fixed.",
  "下面显示的是内置默认值，不是你的设置。这个文件不会被覆盖，所以现在什么都存不进去。": "What is shown below is the built-in defaults, not your settings. The file is never overwritten, so nothing can be saved until it is fixed.",
  "改成": "becomes",
  "正在修…": "Repairing…",
  "备份原文件并修好": "Keep a copy and repair it",

  // 托盘：关窗之后还在不在
  "托盘图标是关掉窗口之后唯一能把它叫回来的地方，所以下面那条挂在它下面。": "The status icon is the only way back to a closed window, which is why the switch below hangs off it.",
  "在托盘显示图标": "Show a status icon in the tray",
  "下次启动时出现": "Appears on the next launch",
  "下次启动时不再出现，这次还在": "Gone from the next launch on; still here for this one",
  "在跑、还是在等你批准，扫一眼图标就知道": "Running, or waiting on you — one glance at the icon says which",
  "关掉窗口后继续在托盘里跑": "Keep running in the tray when the window is closed",
  "会话和后台任务都不会停 —— 从托盘菜单里退出才是真的退出": "Neither the session nor its background jobs stop — quitting from the tray menu is the real quit",
};
