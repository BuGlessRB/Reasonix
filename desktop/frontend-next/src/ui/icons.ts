// The spec names a call by what it is — Search, Update, Read — and derives the
// running line from its category, so a new tool needs no new copy. The raw id
// is still on the row, in the tag beside the name.
const LABEL: Record<string, string> = {
  web_search: "Search", web_fetch: "Fetch", task: "Task", bash: "Bash",
  bash_output: "Bash 输出", kill_shell: "Kill", wait: "Wait",
  read_file: "Read", grep: "Search", glob: "Glob", ls: "List",
  edit_file: "Update", write_file: "Write", multi_edit: "MultiEdit",
  todo_write: "Plan", remember: "Remember", use_capability: "MCP",
  code_index: "Index", complete_step: "Step", review_report: "Review",
  update_goal: "Goal", guardian_assessment: "Guardian", compress: "Compress",
  // Checked against the kernel's registered names, not guessed: these all ship
  // and were rendering as their raw id under a default icon.
  delete_range: "Delete", delete_symbol: "Delete", move_file: "Move",
  notebook_edit: "Notebook", submit_plan: "Plan", ask: "Ask",
  fleet: "Fleet", read_only_task: "Task", read_subagent_result: "Result",
  complete_subtask: "Subtask", lsp_diagnostics: "Diagnostics",
};

const RUNNING: Record<string, string> = {
  plan: "正在写计划…", read: "正在读取…", net: "正在联网…", deleg: "正在派活…",
  bash: "正在执行…", write: "正在改写…", mem: "正在写入记忆…", mcp: "正在调 MCP…",
};

// An MCP tool's raw id repeats its server twice over — the card already carries
// that in its own badge, so the name is just what the tool is called.
export const labelFor = (tool: string) => LABEL[tool] ?? mcpOrigin(tool)?.tool ?? tool;

export function runLabelFor(tool: string) {
  return RUNNING[categoryOf(tool)] ?? "正在处理…";
}

// A tool that changes the tree has to read as one. delete_range, delete_symbol,
// move_file and notebook_edit all mutate and were falling through to the neutral
// "sys" bucket — the colour is the only warning the row carries.
const WRITE = new Set(["delete_range", "delete_symbol", "move_file", "notebook_edit"]);
const DELEG = new Set(["task", "fleet", "read_only_task", "read_subagent_result", "complete_subtask"]);
const READ = new Set(["read_file", "grep", "glob", "ls", "code_index", "lsp_diagnostics"]);

// MCP tools are registered as mcp__<server>__<tool>. Which server answered is
// the one thing a raw id hides and the user needs: it is the difference between
// the agent reading your disk and an external service doing it.
export function mcpOrigin(tool: string): { server: string; tool: string } | null {
  if (!tool.startsWith("mcp__")) return null;
  const at = tool.indexOf("__", 5);
  if (at < 0) return null;
  return { server: tool.slice(5, at), tool: tool.slice(at + 2) };
}

export function categoryOf(tool: string): string {
  if (tool === "web_search" || tool === "web_fetch") return "net";
  if (DELEG.has(tool)) return "deleg";
  if (WRITE.has(tool) || tool.startsWith("edit") || tool.startsWith("write") || tool.startsWith("multi")) return "write";
  if (tool === "use_capability" || tool.startsWith("mcp__")) return "mcp";
  if (tool === "remember") return "mem";
  if (tool === "todo_write" || tool === "submit_plan") return "plan";
  if (tool === "bash" || tool.startsWith("bash_") || tool === "kill_shell") return "bash";
  if (READ.has(tool)) return "read";
  return "sys";
}
