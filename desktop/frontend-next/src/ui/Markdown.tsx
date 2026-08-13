import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import remarkGemoji from "remark-gemoji";
import rehypeRaw from "rehype-raw";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import rehypeKatex from "rehype-katex";
import "katex/dist/katex.min.css";

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
const REHYPE = [rehypeRaw, [rehypeSanitize, schema], rehypeKatex];

// Streaming text arrives mid-token, so an unterminated fence would flip the
// whole tail into a code block for as long as it stays open. Close it for the
// render only; the source string is untouched.
function balanceFences(md: string) {
  const fences = md.match(/^```/gm)?.length ?? 0;
  return fences % 2 === 0 ? md : md + "\n```";
}

export function Markdown({ text, streaming }: { text: string; streaming?: boolean }) {
  return (
    <div className="md">
      <ReactMarkdown
        remarkPlugins={REMARK}
        rehypePlugins={REHYPE as never}
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
        {balanceFences(text)}
      </ReactMarkdown>
      {streaming && <span className="caret" />}
    </div>
  );
}
