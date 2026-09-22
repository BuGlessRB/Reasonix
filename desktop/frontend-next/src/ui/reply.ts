import { useCallback, useMemo, useState } from "react";
import type { AgentPort, Checkpoint } from "../port/port";
import type { Item } from "../state/session";
import { pairCheckpoints } from "../state/checkpoints";
import { t } from "../i18n";
import type { ReplyActions } from "./cards/SayCard";

interface Inputs {
  port: AgentPort;
  items: Item[];
  checkpoints: Checkpoint[];
  running: boolean;
  model?: string;
  submit: (text: string) => Promise<boolean>;
  onSettings: (section?: string) => void;
  onRunDetail: () => void;
  onError: (e: unknown) => void;
}

/** What a finished reply can be acted on with, and the draft signal a quote
 *  travels to the composer on. The transcript owns neither: the pane holds the
 *  session these read from, and the composer is where a quote has to land. */
export function useReplyActions({ port, items, checkpoints, running, model, submit, onSettings, onRunDetail, onError }: Inputs) {
  const [quote, setQuote] = useState<{ text: string; n: number }>({ text: "", n: 0 });

  // Re-running a turn is a conversation rewind and then the same words again:
  // the transcript goes back, the files do not, and the reply that was there
  // stays in the history the rewind wrote. Scope is conversation for exactly
  // that reason — reverting the work too would be a far larger promise than
  // the word "regenerate" makes.
  const regenerate = useCallback(
    async (turn: number, text: string) => {
      try {
        const plan = await port.prepareRewind(turn, "conversation");
        if (!plan.canConversation) throw new Error(plan.disabledReason || t("这一轮无法重新生成"));
        await port.commitRewind(plan.planId);
        await submit(text);
      } catch (e) {
        onError(e);
      }
    },
    [port, submit, onError],
  );

  // The last thing the person said, and the turn the kernel gave it. A reply is
  // re-run by sending that again, so a transcript with no checkpoint behind it
  // offers no regenerate rather than a button that answers with an error. The
  // pairing is the transcript's own: a rebuilt row carries msgIndex and no
  // authored turn, so matching on the latter found nothing in a reopened
  // session — which is most of them.
  const lastAsk = useMemo(() => {
    const paired = pairCheckpoints(items, checkpoints);
    for (let i = items.length - 1; i >= 0; i--) {
      const item = items[i];
      if (item.t !== "user" || item.pending) continue;
      const cp = paired.get(item.id);
      return cp ? { turn: cp.turn, text: item.text } : undefined;
    }
    return undefined;
  }, [items, checkpoints]);

  const reply = useMemo<ReplyActions>(
    () => ({
      onQuote: (text: string) => setQuote((q) => ({ text, n: q.n + 1 })),
      onRegenerate: lastAsk && !running ? () => void regenerate(lastAsk.turn, lastAsk.text) : undefined,
      model,
      onConfigureModel: () => onSettings("model"),
      onRunDetail,
    }),
    [lastAsk, running, regenerate, model, onSettings, onRunDetail],
  );

  // Rewriting a message is the same act with different words: the turn goes
  // back and what the person now means goes out. The card asks for it by the
  // turn its own checkpoint named, so it cannot aim at a turn that moved.
  const onResend = useCallback((turn: number, text: string) => regenerate(turn, text), [regenerate]);

  return { quote, reply, onResend };
}
