import { memo, useEffect, useState, type RefObject } from "react";
import type { Item, Waiting } from "../state/session";
import type { ApprovalVerdict, Checkpoint, RewindPlan, RewindResult, RewindScope } from "../port/port";
import { RMark } from "./RMark";
import { ToolCard } from "./cards/ToolCard";
import { GuardianCard } from "./cards/GuardianCard";
import { ApprovalCard } from "./cards/ApprovalCard";
import { AskCard } from "./cards/AskCard";
import { SayCard } from "./cards/SayCard";
import { CompactionCard } from "./cards/CompactionCard";
import { CompletionCard } from "./cards/CompletionCard";
import { ReadsCard } from "./cards/ReadsCard";
import { UserCard } from "./cards/UserCard";
import { NoticeCard } from "./cards/NoticeCard";
import { RememberCard } from "./cards/RememberCard";
import { ExtensionCard } from "./cards/ExtensionCard";

interface Props {
  items: Item[];
  waiting: Waiting;
  scroll: RefObject<HTMLDivElement | null>;
  hidden: boolean;
  onPinned: (v: boolean) => void;
  onApprove: (itemId: string, id: string, v: ApprovalVerdict) => void;
  onAnswer: (itemId: string, id: string, answers: { questionId: string; selected: string[] }[]) => void;
  onSuggest: (text: string) => void;
  onForget: (itemId: string, name: string) => void;
  onExtInvoke: (name: string) => void;
  onExtSubmit: (pluginId: string, surfaceId: string, values: Record<string, unknown>) => void;
  // The checkpoint each user card can return to, keyed by item id. Absent for a
  // card whose turn could not be matched — see state/checkpoints.
  checkpoints: Map<string, Checkpoint>;
  onPrepareRewind: (turn: number, scope: RewindScope) => Promise<RewindPlan>;
  onCommitRewind: (planId: string) => Promise<RewindResult>;
  onUndoRewind: (transactionId: string) => Promise<void>;
}

export function Transcript({ items, waiting, scroll, hidden, onPinned, onApprove, onAnswer, onSuggest, onForget, onExtInvoke, onExtSubmit, checkpoints, onPrepareRewind, onCommitRewind, onUndoRewind }: Props) {
  // Stick to the bottom only while the reader is already there; scrolling up
  // must not be yanked back by incoming frames.
  const [pinned, setPinned] = useState(true);

  useEffect(() => {
    if (!pinned) return;
    const el = scroll.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [items, pinned, scroll]);

  return (
    <div
      className="scroll"
      id="flowScroll"
      data-pane="flow"
      ref={scroll}
      hidden={hidden}
      onScroll={(e) => {
        const el = e.currentTarget;
        const at = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
        if (at === pinned) return;
        setPinned(at);
        onPinned(at);
      }}
    >
      <div className="flow">
        {items.length === 0 && <Hero onPick={onSuggest} />}
        {items.map((it) => (
          <Row
            key={it.id}
            it={it}
            onApprove={onApprove}
            onAnswer={onAnswer}
            onForget={onForget}
            onExtInvoke={onExtInvoke}
            onExtSubmit={onExtSubmit}
            cp={checkpoints.get(it.id)}
            onPrepareRewind={onPrepareRewind}
            onCommitRewind={onCommitRewind}
            onUndoRewind={onUndoRewind}
          />
        ))}
        {waiting.ttftSince && <Await retry={waiting.retry} />}
      </div>
    </div>
  );
}

// A streamed token replaces one item and leaves the rest identical, so the rest
// must not re-render: at a working session's length that walk cost more per
// chunk than parsing the message did.
const Row = memo(function Row({
  it,
  onApprove,
  onForget,
  onAnswer,
  onExtInvoke,
  onExtSubmit,
  cp,
  onPrepareRewind,
  onCommitRewind,
  onUndoRewind,
}: {
  it: Item;
  onApprove: Props["onApprove"];
  onForget: Props["onForget"];
  onAnswer: Props["onAnswer"];
  onExtInvoke: Props["onExtInvoke"];
  onExtSubmit: Props["onExtSubmit"];
  cp?: Checkpoint;
  onPrepareRewind: Props["onPrepareRewind"];
  onCommitRewind: Props["onCommitRewind"];
  onUndoRewind: Props["onUndoRewind"];
}) {
  switch (it.t) {
    case "user":
      return <UserCard item={it} cp={cp} onPrepareRewind={onPrepareRewind} onCommitRewind={onCommitRewind} onUndoRewind={onUndoRewind} />;
    case "say":
      // Reasoning arrives long before the first answer token on a thinking
      // model. Gating the card on text meant all of it stayed invisible and
      // then landed at once. An empty card is still not a message, so a turn
      // that produced neither draws nothing.
      return it.text.trim() || it.reasoning?.trim() ? <SayCard item={it} /> : null;
    case "tool":
      // The ask tool also raises ask_request, which carries the id /answer
      // needs. Drawing the tool call too put two copies of the same question on
      // screen, each answerable.
      return it.tool.name === "ask" ? null : (
        <ToolCard tool={it.tool} running={it.running} children={it.children} />
      );
    case "reads":
      return <ReadsCard tools={it.tools} />;
    case "guardian":
      return <GuardianCard g={it.g} />;
    case "approval":
      return <ApprovalCard item={it} onApprove={onApprove} />;
    case "ask":
      return <AskCard item={it} onAnswer={onAnswer} />;
    case "compaction":
      return <CompactionCard c={it.c} done={it.done} />;

    case "remember":
      return <RememberCard m={it.m} forgotten={it.forgotten} onForget={(name) => onForget(it.id, name)} />;
    case "completion":
      return <CompletionCard c={it.c} />;
    case "extension":
      return <ExtensionCard ext={it.ext} onInvoke={onExtInvoke} onSubmit={onExtSubmit} />;
    case "notice":
      return <NoticeCard item={it} />;
  }
});

function Await({ retry }: { retry?: { attempt: number; max: number } }) {
  const [secs, setSecs] = useState(0);
  useEffect(() => {
    const t = setInterval(() => setSecs((v) => v + 0.1), 100);
    return () => clearInterval(t);
  }, []);
  return (
    <div className="await" data-retry={retry ? "" : undefined}>
      <i />
      <i />
      <i />
      <span className="t">
        {retry
          ? `连接在响应头前断了，重试 ${retry.attempt}/${retry.max} · ${secs.toFixed(1)}s`
          : `等待回包 ${secs.toFixed(1)}s`}
      </span>
    </div>
  );
}

const SUGGESTIONS = [
  "把这个仓库跑一遍测试，把失败的那几个定位到具体文件",
  "读一遍最近三次提交，告诉我哪里的改动风险最高",
  "查一下这个项目的缓存命中率为什么会掉",
];

function Hero({ onPick }: { onPick: (t: string) => void }) {
  return (
    <div className="hero">
      <RMark />
      <div className="t">交待一件事，它自己往下做</div>
      <div className="s">
        读代码、联网查证、派子代理、改文件 —— 每一步都同时落进「轨迹」，那是机器记录，不是给人读的叙事。
      </div>
      <div className="qs">
        {SUGGESTIONS.map((t) => (
          <button className="sug" key={t} onClick={() => onPick(t)}>
            {t}
          </button>
        ))}
      </div>
    </div>
  );
}
