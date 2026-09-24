import { lazy, Suspense } from "react";
import type { LocalRefs } from "./Markdown";

const Markdown = lazy(async () => ({ default: (await import("./Markdown")).Markdown }));

export function LazyMarkdown({ text, streaming, local }: { text: string; streaming?: boolean; local?: LocalRefs }) {
  return (
    <Suspense fallback={<div className="md" style={{ whiteSpace: "pre-wrap" }}>{text}</div>}>
      <Markdown text={text} streaming={streaming} local={local} />
    </Suspense>
  );
}
