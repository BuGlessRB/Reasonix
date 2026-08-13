import type { Tool } from "../../port/wire";
import type { Ask } from "../../port/wire";
import { AskCard } from "./AskCard";

// Over HTTP the ask tool arrives as a normal tool call whose arguments hold the
// questions; the dedicated AskRequest event only fires on the in-process path.
export function AskFromTool({ tool, onAnswer }: { tool: Tool; onAnswer: (id: string, answers: { questionId: string; selected: string[] }[]) => void }) {
  let ask: Ask | null = null;
  try {
    const parsed = JSON.parse(tool.args ?? "{}");
    if (Array.isArray(parsed.questions)) {
      ask = {
        id: tool.id ?? "ask",
        questions: parsed.questions.map((q: { id?: string; header?: string; question?: string; prompt?: string; multiSelect?: boolean; multi?: boolean; options?: unknown[] }, i: number) => ({
          id: q.id ?? String(i),
          header: q.header ?? "",
          prompt: q.prompt ?? q.question ?? "",
          multi: q.multi ?? q.multiSelect,
          options: (q.options ?? []).map((o) =>
            typeof o === "string" ? { label: o } : (o as { label: string; description?: string }),
          ),
        })),
      };
    }
  } catch {
    ask = null;
  }
  if (!ask) return null;
  return <AskCard item={{ t: "ask", id: tool.id ?? "ask", ask }} onAnswer={onAnswer} />;
}
