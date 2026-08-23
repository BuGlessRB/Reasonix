// What the kernel reports mid-turn, in the reader's language. Keyed by the
// notice's code, which travels on the wire beside the kernel's own English —
// so a code with no entry here still reads, just untranslated. That is what
// makes adding them one at a time safe.
export const NOTICE_TEXT: Record<string, string> = {
  empty_final: "这一轮模型没有给出回答，正在让它重说一次",
  executor_handoff: "模型直接给了答案却没动手，正在要求它先用工具",
};
