import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import remarkGemoji from "remark-gemoji";
import rehypeRaw from "rehype-raw";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";

// Model output is untrusted markup, so raw HTML is parsed and then cut back to
// an allowlist. These four tags carry meaning no markdown syntax expresses;
// nothing that can navigate, load, or script survives the filter. Subscript and
// superscript ride here rather than on a shorthand plugin: Pandoc's ~2~ collides
// with GFM's ~~strikethrough~~, and one mechanism beats two that fight.
const schema = {
  ...defaultSchema,
  tagNames: [...(defaultSchema.tagNames ?? []), "mark", "sub", "sup", "kbd"],
  attributes: {
    ...defaultSchema.attributes,
    // KaTeX marks its output with classes; sanitizing runs before it, so the
    // math wrappers remark-math emits have to survive to be rendered.
    "*": [...(defaultSchema.attributes?.["*"] ?? []), "className"],
  },
};

const REMARK = [remarkGfm, remarkMath, remarkGemoji];
// Order is load-bearing: raw parses HTML into nodes, sanitize prunes them, and
// katex renders afterwards so its generated markup is not pruned in turn.
const BASE_REHYPE = [rehypeRaw, [rehypeSanitize, schema]];

// KaTeX and its fonts are about half the bundle, and most coding sessions never
// produce a formula, so it is fetched the first time one appears. No lookbehind
// — older WebKit treats it as a syntax error and takes the whole page down.
const MATH = /\$\$[\s\S]+?\$\$|\$[^\n$]+\$/;

// Models reach for LaTeX's own delimiters as readily as for dollars, and
// remark-math reads only dollars. Rewriting them keeps one parser instead of
// two. Code is split out first: a \[ inside a fence is text, not an equation.
const CODE = /(```[\s\S]*?```|`[^`\n]*`)/g;

function normalizeMath(md: string) {
  return md
    .split(CODE)
    .map((part, i) =>
      i % 2 === 1
        ? part
        : part
            .replace(/\\\[([\s\S]+?)\\\]/g, (_, m) => `\n\n$$${m}$$\n\n`)
            .replace(/\\\(([\s\S]+?)\\\)/g, (_, m) => `$${m}$`),
    )
    .join("");
}

type Plugin = unknown;
let katex: Plugin | null = null;
let loading: Promise<void> | null = null;

function useKatex(needed: boolean): Plugin | null {
  const [plugin, setPlugin] = useState<Plugin | null>(katex);
  useEffect(() => {
    if (!needed || katex) return;
    let alive = true;
    loading ??= Promise.all([import("rehype-katex"), import("katex/dist/katex.min.css")]).then(
      ([mod]) => {
        // Red source text beats an exception: the message stays readable and
        // the render cannot take the window down with it.
        katex = [mod.default, { throwOnError: false }];
      },
    );
    // A failed chunk leaves the math as source text, which still reads.
    void loading.then(() => alive && setPlugin(() => katex)).catch(() => {});
    return () => {
      alive = false;
    };
  }, [needed]);
  return plugin;
}

// Streaming text arrives mid-token, so an unterminated fence would flip the
// whole tail into a code block for as long as it stays open. Close it for the
// render only; the source string is untouched.
function balanceFences(md: string) {
  const fences = md.match(/^```/gm)?.length ?? 0;
  return fences % 2 === 0 ? md : md + "\n```";
}

export function Markdown({ text, streaming }: { text: string; streaming?: boolean }) {
  const src = normalizeMath(balanceFences(text));
  const math = useKatex(MATH.test(src));
  return (
    <div className="md">
      <ReactMarkdown
        remarkPlugins={REMARK}
        rehypePlugins={(math ? [...BASE_REHYPE, math] : BASE_REHYPE) as never}
        components={{
          // Every link here comes from model output; a webview navigating away
          // would replace the app with the page.
          a: ({ children, href }) => (
            <a href={href} target="_blank" rel="noreferrer noopener">
              {children}
            </a>
          ),
          pre: ({ children }) => <pre className="term">{children}</pre>,
          table: ({ children }) => (
            <div className="md-tw">
              <table>{children}</table>
            </div>
          ),
        }}
      >
        {src}
      </ReactMarkdown>
      {streaming && <span className="caret" />}
    </div>
  );
}
