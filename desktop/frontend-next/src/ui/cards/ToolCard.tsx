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
  // A shell result carries its exit status separately from stdout, and stdout
  // alone cannot say whether the command worked.
  const ex = tool.execution;
  const bad = ex && ((ex.state && ex.state !== "completed") || (ex.exitCode ?? 0) !== 0);
  // The number is the actionable half; the state only says that something went
  // wrong, which the colour already says.
  const badLabel = !ex ? "" : (ex.exitCode ?? 0) !== 0 ? `exit ${ex.exitCode}` : (ex.state ?? "");
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
          {bad && <span className="fail">{badLabel}</span>}
          {cost && <span className="cost">{cost}</span>}
        </div>
        <div className="out">
          {tool.diff && <DiffView diff={tool.diff} path={tool.args} />}
          {!tool.diff && tool.name === "todo_write" && <span className="fold">计划已进右栏</span>}
          {!tool.diff && tool.name !== "todo_write" && tool.output && (
            <Output name={shown} text={tool.output} />
          )}
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
            <Term text={tool.output.slice(0, 400)} />
          </div>
        )}
      </div>
    </div>
  );
}

// read_file numbers every line it returns ("  1→…"), so the count is already
// in the output. A whole file inline buries the rest of the turn, so anything
// long folds behind what it is.
const NUMBERED = /^\s*\d+→/;

function Output({ name, text }: { name: string; text: string }) {
  const numbered = text.split("\n").filter((l) => NUMBERED.test(l)).length;
  if (name !== "read_file" || numbered <= 12) return <Term text={text} />;
  return (
    <details>
      <summary>
        <span className="fold">读了 {numbered} 行</span>
      </summary>
      <Term text={text} />
    </details>
  );
}

// The terminal fills in line by line: the first few land on the beat, the rest
// tighten up so a long block still finishes inside SETTLED.
const HEAD = 5;
const BEAT = 34;
const TIGHT = 8;
const SETTLED = 700;
// Past this, splitting into one node per line costs more than the entrance is
// worth, so the block arrives whole.
const SPLIT_MAX = 300;

function Term({ text }: { text: string }) {
  const lines = text.split("\n");
  if (lines.length > SPLIT_MAX) return <pre className="term">{text}</pre>;
  return (
    <pre className="term">
      {lines.map((l, i) => (
        <span
          className="term-l"
          key={i}
          style={{ animationDelay: `${Math.min(i < HEAD ? i * BEAT : HEAD * BEAT + (i - HEAD) * TIGHT, SETTLED)}ms` }}
        >
          {l}
          {i < lines.length - 1 ? "\n" : ""}
        </span>
      ))}
    </pre>
  );
}

// Only the categories the spec gives a colour to; the rest stay neutral.
const KINDED = new Set(["net", "deleg", "write", "mcp", "mem"]);
