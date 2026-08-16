// Every shape here was read off real tool results, not inferred: a search hit
// is "path:line:text", a context line repeats the path with a dash, a summary
// is "path: N matches", ls is "name<TAB>bytes". Anything that does not parse
// cleanly falls back to the terminal block rather than being forced into a
// list — a wrong list reads as authoritative, a raw block only reads as raw.

import { splitPath } from "../args";

const MATCH = /^(.+?):(\d+):([\s\S]*)$/;
// ls prints a directory as "name/" with no size and a file as "name<TAB>bytes".
const LS_DIR = /^(.+\/)$/;
const LS_FILE = /^(.+)\t(\d+)$/;

// Below this share of lines fitting the shape, the output is something else.
const CONFIDENT = 0.8;
// A search can return hundreds; the rest are counted, never silently dropped.
const ROWS = 40;

export interface Hit {
  path: string;
  line: string;
  text: string;
  more: string[];
}

export function parseHits(out: string): { hits: Hit[]; note: string } | null {
  const all = out.split("\n").filter((l) => l.trim());
  // grep appends its own truncation and timeout notes. They are the tool
  // telling you the result is partial, not stray output — pull them out before
  // judging the shape, and keep them to show.
  const note = all.filter((l) => TRAILER.test(l)).join(" ");
  const lines = all.filter((l) => !TRAILER.test(l));
  if (lines.length === 0) return null;
  const hits: Hit[] = [];
  let loose = 0;
  for (const line of lines) {
    const m = MATCH.exec(line);
    if (m) {
      hits.push({ path: m[1], line: m[2], text: m[3].trim(), more: [] });
      continue;
    }
    const cur = hits[hits.length - 1];
    // A context line repeats the hit's own path, so strip it by literal rather
    // than by a pattern that a path containing "-12-" would fool.
    if (cur && line.startsWith(cur.path)) {
      cur.more.push(line.slice(cur.path.length).replace(/^[-:]\d+[-:]/, "").trim());
      continue;
    }
    loose++;
  }
  return hits.length && loose / lines.length <= 1 - CONFIDENT ? { hits, note } : null;
}

export interface Row {
  name: string;
  n: string;
}

export function parseRows(out: string, re: RegExp): Row[] | null {
  const lines = out.split("\n").filter((l) => l.trim());
  if (lines.length === 0) return null;
  const rows = lines.map((l) => re.exec(l)).filter((m): m is RegExpExecArray => m !== null);
  if (rows.length / lines.length < CONFIDENT) return null;
  // A shape with no count still makes a manifest; the column just stays empty.
  return rows.map((m) => ({ name: m[1], n: m[2] ?? "" }));
}

export function parseListing(out: string): Row[] | null {
  const lines = out.split("\n").filter((l) => l.trim());
  if (lines.length === 0) return null;
  const rows: Row[] = [];
  for (const l of lines) {
    const f = LS_FILE.exec(l);
    if (f) {
      rows.push({ name: f[1], n: f[2] });
      continue;
    }
    const d = LS_DIR.exec(l);
    if (d) rows.push({ name: d[1], n: "" });
  }
  return rows.length / lines.length < CONFIDENT ? null : rows;
}

// A manifest is read-only where the spec draws it. Only a merged read run makes
// its rows pickable, because collapsing the cards would otherwise put the file
// contents out of reach entirely.
export function Peek({
  rows,
  unit,
  onPick,
  at,
}: {
  rows: { name: string; n: string }[];
  unit: string;
  onPick?: (i: number) => void;
  at?: number;
}) {
  const shown = rows.slice(0, ROWS);
  return (
    <div className="peek">
      {shown.map((r, i) => {
        const [dir, name] = splitPath(r.name);
        const body = (
          <>
            <span>
              {dir && <span className="d">{dir}</span>}
              {name}
            </span>
            {r.n && (
              <span className="n">
                {r.n}
                {unit && ` ${unit}`}
              </span>
            )}
          </>
        );
        return onPick ? (
          <button className="row" key={i} data-on={at === i ? "" : undefined} onClick={() => onPick(i)}>
            {body}
          </button>
        ) : (
          <div className="row" key={i}>
            {body}
          </div>
        );
      })}
      {rows.length > shown.length && (
        <div className="row">
          <span className="d">还有 {rows.length - shown.length} 条</span>
        </div>
      )}
    </div>
  );
}

// One result row: what it is, where it came from, what it said. A grep hit and a
// web search result are the same shape — a claim with a source — so the spec
// draws them with one set of elements rather than two lookalike lists.
export interface Row3 {
  t: string;
  u: string;
  q: string[];
}

export const hitRows = (hits: Hit[]): Row3[] =>
  hits.map((h) => {
    const [dir, name] = splitPath(h.path);
    return { t: name, u: `${dir}${name}:${h.line}`, q: [h.text, ...h.more] };
  });

export function Hits({ rows, note, unit = "条" }: { rows: Row3[]; note?: string; unit?: string }) {
  const shown = rows.slice(0, ROWS);
  return (
    <div className="hits">
      {shown.map((r, i) => (
        <div className="hit-row" key={i}>
          <div className="i">{i + 1}</div>
          {/* .t/.u/.q carry margin-top and ellipsis but no display in the
              spec's CSS, so they were block elements in the prototype. */}
          <div>
            <div className="t">{r.t}</div>
            {r.u && (
              <div className="u" title={r.u}>
                {r.u}
              </div>
            )}
            {/* A provider that encrypts result bodies leaves nothing to quote,
                and an empty .q still draws the spec's quote rule. */}
            {r.q.some(Boolean) && (
              <div className="q">
                {r.q[0]}
                {r.q.slice(1).map((m, k) => (
                  <div key={k}>{m}</div>
                ))}
              </div>
            )}
          </div>
        </div>
      ))}
      {(rows.length > shown.length || note) && (
        <div className="hit-row">
          <div className="i" />
          <div className="u">
            {rows.length > shown.length && `还有 ${rows.length - shown.length} ${unit}`}
            {rows.length > shown.length && note && " · "}
            {note}
          </div>
        </div>
      )}
    </div>
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

export function Term({ text }: { text: string }) {
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

// A provider-run search returns the listing the kernel formats for the model:
// "- **title**" and, indented under it, "<url>". A result can arrive with no URL
// at all, so the title alone still counts as a row.
const SEARCH_TITLE = /^-\s+\*\*(.+?)\*\*\s*$/;
const SEARCH_URL = /^\s+<(\S+)>\s*$/;

export function parseSearchResults(out: string): Row3[] | null {
  const lines = out.split("\n").filter((l) => l.trim());
  if (lines.length === 0) return null;
  const rows: Row3[] = [];
  let loose = 0;
  for (const line of lines) {
    const title = SEARCH_TITLE.exec(line);
    if (title) {
      rows.push({ t: title[1], u: "", q: [] });
      continue;
    }
    const url = SEARCH_URL.exec(line);
    const cur = rows[rows.length - 1];
    if (url && cur && !cur.u) {
      cur.u = url[1];
      continue;
    }
    loose++;
  }
  return rows.length && loose / lines.length <= 1 - CONFIDENT ? rows : null;
}

// read_file numbers every line it returns ("  1→…"), so the count is already in
// the output. A whole file inline buries the rest of the turn, so anything long
// folds behind what it is.
const NUMBERED = /^\s*\d+→/;
// glob returns one path per line. Requiring a separator keeps a one-line error
// message from being dressed up as a manifest of one file.
export const PATH = /^\s*([^\s]*[/\\][^\s]*)\s*$/;
const TRAILER = /^\.\.\. \(/;

export function ToolOutput({ name, text }: { name: string; text: string }) {
  if (name === "read_file") {
    const numbered = text.split("\n").filter((l) => NUMBERED.test(l)).length;
    if (numbered <= 12) return <Term text={text} />;
    return (
      <details>
        <summary>
          <span className="fold">读了 {numbered} 行</span>
        </summary>
        <Term text={text} />
      </details>
    );
  }
  // A directory listing and a glob result are both manifests, which is what
  // .peek is for. grep prints "path:line:text" and gets the excerpt list.
  if (name === "ls") {
    const rows = parseListing(text);
    if (rows) return <Peek rows={rows} unit="B" />;
  }
  if (name === "glob") {
    const rows = parseRows(text, PATH);
    if (rows) return <Peek rows={rows} unit="" />;
  }
  if (name === "grep") {
    const found = parseHits(text);
    if (found) return <Hits rows={hitRows(found.hits)} note={found.note} unit="处匹配" />;
  }
  if (name === "web_search") {
    const rows = parseSearchResults(text);
    if (rows) return <Hits rows={rows} />;
  }
  return <Folded text={text} />;
}

// Anything without a shape of its own: a shell command, an MCP server's result,
// a listing that did not parse. Forty lines of build log buries the rest of the
// turn exactly the way a whole file did, so it folds for the same reason — with
// the first line kept on the summary, because that is usually the answer.
// 4, not read_file's 12: a shell result's median is three lines, and folding
// those costs a click to read what a glance already had. Past four it is worth
// the fold — a whole file is long by nature, a command's answer usually is not.
const FOLD_LINES = 4;
const HEAD_CHARS = 48;

function Folded({ text }: { text: string }) {
  const lines = text.split("\n");
  if (lines.length <= FOLD_LINES) return <Term text={text} />;
  // MCP results are JSON, whose first line is "{" — a summary of one brace
  // tells you nothing. Take the first line that carries a word.
  const head = lines.find((l) => /[\p{L}\p{N}]/u.test(l))?.trim() ?? "";
  return (
    <details>
      <summary>
        <span className="fold">
          {lines.length} 行输出
          {head && ` · ${head.length > HEAD_CHARS ? head.slice(0, HEAD_CHARS - 1) + "…" : head}`}
        </span>
      </summary>
      <Term text={text} />
    </details>
  );
}
