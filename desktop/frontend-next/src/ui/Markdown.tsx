import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

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
        remarkPlugins={[remarkGfm]}
        components={{
          // Every link here comes from model output; a webview navigating away
          // would replace the app with the page.
          a: ({ children, href }) => (
            <a href={href} target="_blank" rel="noreferrer noopener">
              {children}
            </a>
          ),
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
