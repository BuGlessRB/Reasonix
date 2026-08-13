import { useEffect, useRef, useState } from "react";
import type { Tool } from "../../port/wire";
import { categoryOf, labelFor, mcpOrigin, runLabelFor } from "../icons";
import { Sym, glyphFor } from "../Sym";
import { shortArgs } from "../args";
import { DiffView } from "./DiffView";
import { Term, ToolOutput } from "./ToolOutput";

// The spec pops a symbol as it settles — colour arriving is the finish signal.
// Only the transition may fire it: a restored transcript is all settled cards,
// and marking them done would pop the whole page on load.
// The head crossfades before it swaps: "「正在读取…」→「读了 7 个文件」是这一步
// 唯一的状态跃迁，别用 0ms 换掉它". 90ms out, then the settled text.
const SWAP = 90;

function useSettling(running: boolean): { pop: boolean; swap: boolean } {
  const [pop, setPop] = useState(false);
  const [swap, setSwap] = useState(false);
  const was = useRef(running);
  useEffect(() => {
    const fell = was.current && !running;
    was.current = running;
    if (!fell) return;
    setPop(true);
    setSwap(true);
    const a = setTimeout(() => setSwap(false), SWAP);
    const b = setTimeout(() => setPop(false), 340);
    return () => {
      clearTimeout(a);
      clearTimeout(b);
    };
  }, [running]);
  return { pop, swap };
}

export function ToolCard({ tool, running, children = [] }: { tool: Tool; running: boolean; children?: Tool[] }) {
  // use_capability is the proxy the model reaches a capability through, not the
  // thing it did: name the resolved tool and keep the proxy in the tag.
  const shown = tool.resolvedName || tool.name;
  const cost = tool.durationMs ? `${(tool.durationMs / 1000).toFixed(1)}s` : "";
  // Running and settled are two different lines: the category says what it is
  // doing now, the label says what it was once it is done.
  const head = running ? runLabelFor(shown) : labelFor(shown);
  const arg = tool.name === "todo_write" ? "" : shortArgs(tool.args ?? "");
  // A shell result carries its exit status separately from stdout, and stdout
  // alone cannot say whether the command worked.
  const ex = tool.execution;
  const bad = ex && ((ex.state && ex.state !== "completed") || (ex.exitCode ?? 0) !== 0);
  // The number is the actionable half; the state only says that something went
  // wrong, which the colour already says.
  const badLabel = !ex ? "" : (ex.exitCode ?? 0) !== 0 ? `exit ${ex.exitCode}` : (ex.state ?? "");
  // Which server answered belongs on the card, not in a panel: this is the
  // moment the user can judge whether an external service should have run.
  const from = mcpOrigin(shown);
  const invoked = mcpOrigin(tool.name);
  // A delegated step is only auditable if it names the profile that ran: "a
  // subagent" is not a thing you can go read, "skill:security-review" is.
  const who = tool.profile?.name?.trim();
  const settling = useSettling(running);
  return (
    <div className="call" data-k={KINDED.has(categoryOf(shown)) ? categoryOf(shown) : undefined} data-running={running ? "" : undefined}>
      <div className="g">
        <Sym glyph={glyphFor(shown)} done={settling.pop} />
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl" data-swap={settling.swap ? "" : undefined}>
          <span className={running ? "nm shim" : "nm"}>{head}</span>
          {who && <span className="who" title={`按 ${who} 这份技能的设定跑的子代理`}>{who}</span>}
          {from && <span className="src" title={`外部服务 ${from.server} 提供的工具`}>{from.server}</span>}
          <span className="tag">{invoked ? invoked.tool : tool.name}</span>
          {arg && <span className="arg">{arg}</span>}
          {bad && <span className="fail">{badLabel}</span>}
          {cost && <span className="cost">{cost}</span>}
        </div>
        <div className="out">
          {tool.diff && <DiffView diff={tool.diff} path={tool.args} />}
          {!tool.diff && tool.name === "todo_write" && <span className="fold">计划已进右栏</span>}
          {!tool.diff && tool.name !== "todo_write" && tool.output && (
            <ToolOutput name={shown} text={tool.output} />
          )}
          {tool.err && <div className="txt">{tool.err}</div>}
          {children.length > 0 && (
            <div className="nest">
              <div className="nest-hd">
                <i className="pip" />
                <span className="who">{who ? `${who} 做了 ${children.length} 步` : `${children.length} 个子代理`}</span>
                <span className="prof">独立上下文 · 不进主轨迹</span>
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
  return (
    <div className="call">
      <div className="g">
        <Sym glyph={glyphFor(tool.name)} />
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className="nm">{tool.name}</span>
          {tool.args && <span className="arg">{shortArgs(tool.args)}</span>}
        </div>
        {tool.output && (
          <div className="out">
            <Term text={tool.output.slice(0, 400)} />
          </div>
        )}
      </div>
    </div>
  );
}

// Only the categories the spec gives a colour to; the rest stay neutral.
const KINDED = new Set(["net", "deleg", "write", "mcp", "mem"]);
