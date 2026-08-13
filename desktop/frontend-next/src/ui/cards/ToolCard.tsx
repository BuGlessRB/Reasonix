import type { Tool } from "../../port/wire";
import { ICON, symFor } from "../icons";
import { DiffView } from "./DiffView";

export function ToolCard({ tool, running, children = [] }: { tool: Tool; running: boolean; children?: Tool[] }) {
  const sym = symFor(tool.name);
  const cost = tool.durationMs ? `${(tool.durationMs / 1000).toFixed(1)}s` : "";
  const arg = tool.name === "todo_write" ? "" : shortArgs(tool.args ?? "");
  return (
    <div className="call" data-k={kindOf(tool.name)} data-running={running ? "" : undefined}>
      <div className="g">
        <span className="sym">
          <svg viewBox="0 0 16 16" aria-hidden="true">
            <path d={ICON[sym]} />
          </svg>
        </span>
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className={running ? "nm shim" : "nm"}>{running ? "正在执行…" : tool.name}</span>
          <span className="tag">{tool.name}</span>
          {arg && <span className="arg">{arg}</span>}
          {cost && <span className="cost">{cost}</span>}
        </div>
        <div className="out">
          {tool.diff && <DiffView diff={tool.diff} path={tool.args} />}
          {!tool.diff && tool.name === "todo_write" && <span className="fold">计划已进右栏</span>}
          {!tool.diff && tool.name !== "todo_write" && tool.output && <pre className="term">{tool.output}</pre>}
          {tool.err && <div className="txt">{tool.err}</div>}
          {children.length > 0 && (
            <div className="nest">
              <div className="nest-hd">
                <i className="pip" />
                <span className="who">{children.length} 个子代理</span>
                <span className="prof">具名 skill · 独立轨迹</span>
              </div>
              <div className="nest-bd">
                {children.map((c) => (
                  <div className="call" key={c.id}>
                    <div className="g">
                      <span className="sym">
                        <svg viewBox="0 0 16 16" aria-hidden="true">
                          <path d={ICON[symFor(c.name)]} />
                        </svg>
                      </span>
                      <span className="line" />
                    </div>
                    <div className="c">
                      <div className="hl">
                        <span className="nm">{c.name}</span>
                        {c.args && <span className="arg">{shortArgs(c.args)}</span>}
                      </div>
                      {c.output && (
                        <div className="out">
                          <pre className="term">{c.output.slice(0, 400)}</pre>
                        </div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

const ARG_KEYS = ["command", "path", "file_path", "pattern", "query", "url", "description", "prompt", "step_id"];

function shortArgs(raw: string) {
  if (!raw) return "";
  try {
    const v = JSON.parse(raw);
    for (const k of ARG_KEYS) {
      if (typeof v[k] === "string" && v[k]) return v[k].replace(/\s+/g, " ").slice(0, 96);
    }
    return "";
  } catch {
    return raw.replace(/\s+/g, " ").slice(0, 96);
  }
}

function kindOf(name: string) {
  if (name === "web_search" || name === "web_fetch") return "net";
  if (name === "task") return "deleg";
  if (name.startsWith("edit") || name.startsWith("write") || name.startsWith("multi")) return "write";
  if (name === "use_capability") return "mcp";
  if (name === "remember") return "mem";
  return "";
}
