import {
  Bookmark,
  Check,
  CircleHelp,
  FilePen,
  FileText,
  Folder,
  FolderSearch,
  GitBranch,
  Globe,
  Hash,
  ListChecks,
  Plug,
  ScanSearch,
  Search,
  ShieldCheck,
  Shrink,
  SquareCheck,
  SquareTerminal,
  Target,
  type LucideIcon,
} from "lucide-react";

const BY_TOOL: Record<string, LucideIcon> = {
  web_search: Globe,
  web_fetch: Globe,
  task: GitBranch,
  edit_file: FilePen,
  write_file: FilePen,
  multi_edit: FilePen,
  use_capability: Plug,
  remember: Bookmark,
  bash: SquareTerminal,
  bash_output: SquareTerminal,
  kill_shell: SquareTerminal,
  wait: SquareTerminal,
  read_file: FileText,
  grep: Search,
  glob: FolderSearch,
  ls: Folder,
  todo_write: ListChecks,
  code_index: Hash,
  complete_step: SquareCheck,
  compress: Shrink,
  update_goal: Target,
  review_report: ScanSearch,
  guardian_assessment: ShieldCheck,
  ask: CircleHelp,
};

export const iconFor = (tool: string): LucideIcon => BY_TOOL[tool] ?? FileText;

export const DONE = Check;

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
};

const RUNNING: Record<string, string> = {
  plan: "正在写计划…", read: "正在读取…", net: "正在联网…", deleg: "正在派活…",
  bash: "正在执行…", write: "正在改写…", mem: "正在写入记忆…", mcp: "正在调 MCP…",
};

export const labelFor = (tool: string) => LABEL[tool] ?? tool;

export function runLabelFor(tool: string) {
  return RUNNING[categoryOf(tool)] ?? "正在处理…";
}

export function categoryOf(tool: string): string {
  if (tool === "web_search" || tool === "web_fetch") return "net";
  if (tool === "task") return "deleg";
  if (tool.startsWith("edit") || tool.startsWith("write") || tool.startsWith("multi")) return "write";
  if (tool === "use_capability") return "mcp";
  if (tool === "remember") return "mem";
  if (tool === "todo_write") return "plan";
  if (tool === "bash" || tool.startsWith("bash_") || tool === "kill_shell") return "bash";
  if (tool === "read_file" || tool === "grep" || tool === "glob" || tool === "ls") return "read";
  return "sys";
}
