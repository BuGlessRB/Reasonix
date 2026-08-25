// The run graph: the shape a turn's fan-out actually had. Its own catalogue for
// the same reason the others have one — en.ts sits at the file-size ceiling, and
// a screen's worth of wording is what a translator reads at a time.
export const EN_GRAPH: Record<string, string> = {
  "图": "Graph",
  "在活动流中定位": "Find it in the activity stream",
  "这一轮没有派出并行的子代理。fleet 或 parallel_tasks 一开跑，它们的形状就画在这里 —— 谁在等谁，谁在同时跑，哪些答案是复用的。":
    "No sub-agents were dispatched in parallel this turn. The moment a fleet or parallel_tasks starts, its shape is drawn here: what is waiting on what, what is running at the same time, and which answers were reused rather than paid for again.",
  "· {n} 个没有重跑": "· {n} not re-run",
  "· {n} 个在等上游": "· {n} waiting upstream",
  "依赖 · 答案已交付": "Depends · answer delivered",
  "只排了序": "Ordered only",
  "复用了已完成的结果": "Reused a finished answer",
  "{n} 个跑了": "{n} ran",
  "· {n} 个复用": "· {n} reused",
  "等 {who}": "waiting on {who}",
  "等 {n} 项上游": "waiting on {n} upstream",
  "待运行": "Not started",
  "复用": "Reused",
  "已取消": "Cancelled",
  "跳过": "Skipped",
  "状态": "State",
  "画像": "Profile",
  "权限": "Grant",
  "可写": "Writes",
  "耗时": "Took",
  "等待": "Waiting on",
  "复用自": "Reused from",
  "记录": "Transcript",
  "错误": "Error",
};
