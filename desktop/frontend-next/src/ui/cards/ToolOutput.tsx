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

export function Hits({ hits, note }: { hits: Hit[]; note?: string }) {
  const shown = hits.slice(0, ROWS);
  return (
    <div className="hits">
      {shown.map((h, i) => {
        const [dir, name] = splitPath(h.path);
        return (
          <div className="hit-row" key={i}>
            <div className="i">{i + 1}</div>
            {/* .t/.u/.q carry margin-top and ellipsis but no display in the
                spec's CSS, so they were block elements in the prototype. */}
            <div>
              <div className="t">{name}</div>
              <div className="u" title={h.path}>
                {dir}
                {name}:{h.line}
              </div>
              <div className="q">
                {h.text}
                {h.more.map((m, k) => (
                  <div key={k}>{m}</div>
                ))}
              </div>
            </div>
          </div>
        );
      })}
      {(hits.length > shown.length || note) && (
        <div className="hit-row">
          <div className="i" />
          <div className="u">
            {hits.length > shown.length && `还有 ${hits.length - shown.length} 处匹配`}
            {hits.length > shown.length && note && " · "}
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

// read_file numbers every line it returns ("  1→…"), so the count is already in
// the output. A whole file inline buries the rest of the turn, so anything long
// folds behind what it is.
const NUMBERED = /^\s*\d+→/;
// glob returns one path per line. Requiring a separator keeps a one-line error
// message from being dressed up as a manifest of one file.
const PATH = /^\s*([^\s]*[/\\][^\s]*)\s*$/;
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
    if (found) return <Hits hits={found.hits} note={found.note} />;
  }
  return <Term text={text} />;
}
