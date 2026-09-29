import { useEffect, useRef, useState } from "react";
import { readDraft, writeDraft } from "./drafts";

export function useDraft(draftKey: string, submitting: boolean) {
  const [text, setText] = useState(() => readDraft(draftKey));
  const keyRef = useRef(draftKey);
  const textRef = useRef(text);
  const pendingRef = useRef<string | null>(null);
  const skipWrite = useRef(false);
  textRef.current = text;

  useEffect(() => {
    if (keyRef.current === draftKey) return;
    const previous = keyRef.current;
    writeDraft(previous, textRef.current);
    keyRef.current = draftKey;
    if (!draftKey) return;
    const saved = readDraft(draftKey);
    if (!previous && !saved && textRef.current) writeDraft(draftKey, textRef.current);
    else setText(saved);
    skipWrite.current = true;
  }, [draftKey]);

  useEffect(() => {
    if (!draftKey || skipWrite.current || submitting) {
      skipWrite.current = false;
      return;
    }
    const timer = window.setTimeout(() => writeDraft(draftKey, text), 250);
    return () => window.clearTimeout(timer);
  }, [draftKey, text, submitting]);

  useEffect(() => {
    const flush = () => writeDraft(keyRef.current, pendingRef.current ?? textRef.current);
    window.addEventListener("pagehide", flush);
    return () => {
      window.removeEventListener("pagehide", flush);
      flush();
    };
  }, []);

  return {
    text,
    setText,
    beginSubmit: (draft: string) => { pendingRef.current = draft; },
    finishSubmit: (sent: boolean) => {
      pendingRef.current = null;
      if (sent) {
        textRef.current = "";
        writeDraft(keyRef.current, "");
      }
    },
  };
}
