import type { Tool } from "../../port/wire";
import { categoryOf, iconFor, labelFor, runLabelFor } from "../icons";
import { shortArgs } from "../args";
import { DiffView } from "./DiffView";

export function ToolCard({ tool, running, children = [] }: { tool: Tool; running: boolean; children?: Tool[] }) {
  // use_capability is the proxy the model reaches a capability through, not the
  // thing it did: name the resolved tool and keep the proxy in the tag.
  const shown = tool.resolvedName || tool.name;
  const Sym = iconFor(shown);
  const cost = tool.durationMs ? `${(tool.durationMs / 1000).toFixed(1)}s` : "";
  // Running and settled are two different lines: the category says what it is
  // doing now, the label says what it was once it is done.
  const head = running ? runLabelFor(shown) : labelFor(shown);
  const arg = tool.name === "todo_write" ? "" : shortArgs(tool.args ?? "");
  return (
    <div className="call" data-k={KINDED.has(categoryOf(shown)) ? categoryOf(shown) : undefined} data-running={running ? "" : undefined}>
      <div className="g">
        <span className="sym">
          <Sym aria-hidden="true" />
        </span>
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className={running ? "nm shim" : "nm"}>{head}</span>
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
                  <NestedCall key={c.id} tool={c} />
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function NestedCall({ tool }: { tool: Tool }) {
  const Sym = iconFor(tool.name);
  return (
    <div className="call">
      <div className="g">
        <span className="sym">
          <Sym aria-hidden="true" />
        </span>
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className="nm">{tool.name}</span>
          {tool.args && <span className="arg">{shortArgs(tool.args)}</span>}
        </div>
        {tool.output && (
          <div className="out">
            <pre className="term">{tool.output.slice(0, 400)}</pre>
          </div>
        )}
      </div>
    </div>
  );
}

// Only the categories the spec gives a colour to; the rest stay neutral.
const KINDED = new Set(["net", "deleg", "write", "mcp", "mem"]);
