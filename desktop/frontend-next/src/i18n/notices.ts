// What the kernel reports mid-turn, in the reader's language. Keyed by the
// notice's code, which travels on the wire beside the kernel's own English —
// so a code with no entry here still reads, just untranslated. That is what
// makes adding them one at a time safe.
export const NOTICE_TEXT: Record<string, string> = {
  empty_final: "这一轮模型没有给出回答，正在让它重说一次",
  executor_handoff: "模型直接给了答案却没动手，正在要求它先用工具",
  workspace_lease: "另一个会话正在写这个工作区，等它写完会自动继续",
  workspace_lease_resumed: "工作区空出来了，这个会话已经继续",
  workspace_lease_abandoned: "还没轮到这个会话写，这次等待就结束了",
};
