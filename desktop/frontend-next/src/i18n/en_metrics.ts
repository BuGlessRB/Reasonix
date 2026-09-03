// The metrics rail: what a turn is costing, what filled the window, and what it
// touched outside the transcript. Split out for the same reason en_settings and
// en_kernel were — one screen's worth of wording, read together.
export const EN_METRICS: Record<string, string> = {
  "{n} 在跑": "{n} running",
  "前缀缓存": "Prefix cache",
  "工作树改动": "Worktree changes",
  "没有在跑的后台任务": "No background jobs running",
  "已放行": "auto-approved",
  "待审": "for review",
  "前缀变了": "prefix changed",
  "前缀未变": "prefix held",
  "正文变了": "body changed",
  "正文未变": "body held",
  "沿用的消息条数": "Messages carried over",
  "前缀哈希": "Prefix hash",
  "按兜底价估": "fallback price",
  "已结算": "settled",
  "本回合": "this turn ",
  "原币种": "as quoted",
  "两种结算币不相加 —— 合成一个总数就得凭空发明一个汇率。": "Two settlement currencies do not add up — one total would mean inventing a rate.",
  "工具 schema": "Tool schema",
  "远程主机": "Remote hosts",
  "还没有请求": "no requests yet",
  "没人说过这个来源的窗口有多大，所以画不出用了多少 —— 也不会自动压缩。中转站转发的是别人的模型，只有你知道它有多大。":
    "Nobody has said how large this source's window is, so there is no share to draw — and no automatic compaction either. A relay forwards somebody else's model, so only you know what it holds.",
  "记下": "Save",
  "这个窗口是谁填的说不准 —— 点一下改成这个模型真正的上限":
    "Whoever entered this window may not have meant this model — click to set what it actually holds",
  "只改当前这个模型，同一个来源下的其它模型不动。填模型文档写的上下文上限，不是最大输出。会重建运行时，任务跑着的时候改不了。":
    "Applies to this model alone; the others on this source are left as they are. The context ceiling from the model's docs, not its max output. This rebuilds the runtime, so it cannot be set while a task is running.",
  "填模型文档写的上下文上限，不是最大输出。会重建运行时，任务跑着的时候改不了。":
    "The context ceiling from the model's docs, not its max output. This rebuilds the runtime, so it cannot be set while a task is running.",
  "{n} 会话": "{n} sessions",
  "每回合用量": "Tokens per round",
  "峰值 {peak} · 均 {avg}": "peak {peak} · avg {avg}",
  "正在读这个文件的改动…": "Reading this file's changes…",
  "这个文件现在和上一次提交一样": "This file now matches the last commit",
  "改动太长，只显示了前面一段": "Too long to show in full — this is the start of it",
  "改了 {n} 次": "edited {n} times",
  "改过": "Modified",
};
