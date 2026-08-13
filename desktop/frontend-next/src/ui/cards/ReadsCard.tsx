import { useState } from "react";
import type { Tool } from "../../port/wire";
import { argOf } from "../args";
import { labelFor } from "../icons";
import { Sym, glyphFor } from "../Sym";
import { Peek, Term } from "./ToolOutput";

// read_file numbers every line it returns, so the count is in the output rather
// than guessed from it.
const NUMBERED = /^\s*\d+→/;
const lines = (out?: string) => (out ? out.split("\n").filter((l) => NUMBERED.test(l)).length : 0);

// A run of reads is one step, not one card each. The manifest answers what was
// read; the file itself is one click away rather than dumped into the flow.
export function ReadsCard({ tools }: { tools: Tool[] }) {
  const [open, setOpen] = useState(-1);
  const rows = tools.map((t) => ({ name: argOf(t.args, "path", "file_path") || t.args || "—", n: String(lines(t.output)) }));
  const total = tools.reduce((n, t) => n + lines(t.output), 0);
  return (
    <div className="call">
      <div className="g">
        <Sym glyph={glyphFor("read_file")} />
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className="nm">{labelFor("read_file")}</span>
          <span className="tag">read_file</span>
          <span className="arg">
            {tools.length} 个文件 · {total} 行
          </span>
        </div>
        <div className="out">
          <Peek rows={rows} unit="行" onPick={(i) => setOpen(i === open ? -1 : i)} at={open} />
          {open >= 0 && tools[open]?.output && <Term text={tools[open].output} />}
        </div>
      </div>
    </div>
  );
}
