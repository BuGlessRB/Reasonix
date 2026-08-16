import { memo, useCallback, useEffect, useLayoutEffect, useRef, useState, type RefObject } from "react";
import { t } from "../i18n";
import type { Item, Waiting } from "../state/session";
import type { ExtensionSurface } from "../port/wire";
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
import { Rail, type RailMark } from "./Rail";

interface Props {
  items: Item[];
  // Changes only when the transcript's composition does — see state/session.
  revision: number;
  waiting: Waiting;
  scroll: RefObject<HTMLDivElement | null>;
  hidden: boolean;
  onPinned: (v: boolean) => void;
  // Bumped to ask for the bottom back — see the effect that consumes it.
  jump: number;
  onApprove: (itemId: string, id: string, v: ApprovalVerdict) => void;
  onAnswer: (itemId: string, id: string, answers: { questionId: string; selected: string[] }[]) => void;
  onSuggest: (text: string) => void;
  onForget: (itemId: string, name: string) => void;
  onExtInvoke: (name: string) => void;
  // Views published against a tool call, keyed by anchor. A card looks itself
  // up here rather than being handed one, so an arriving takeover repaints the
  // one card it names and nothing else.
  takeovers?: Record<string, ExtensionSurface>;
  onExtSubmit: (pluginId: string, surfaceId: string, values: Record<string, unknown>) => void;
  // The checkpoint each user card can return to, keyed by item id. Absent for a
  // card whose turn could not be matched — see state/checkpoints.
  checkpoints: Map<string, Checkpoint>;
  onPrepareRewind: (turn: number, scope: RewindScope) => Promise<RewindPlan>;
  onCommitRewind: (planId: string) => Promise<RewindResult>;
  onUndoRewind: (transactionId: string) => Promise<void>;
}

// How many settled cards share one mounting unit. Small enough that scrolling
// mounts a block within a frame, large enough that a long session holds a few
// hundred blocks rather than tens of thousands.
const BLOCK = 48;

// A streamed delta rewrites the last card and leaves every earlier one at the
// same identity, so the settled head is cut into blocks that keep their array
// identity across frames — that is what lets each block memo instead of being
// rebuilt on every chunk of text.
//
// Revision and length decide when to re-cut: anything that edits a card in the
// middle — a tool settling, an approval being answered — bumps the revision by
// construction, and a delta that opens a new message changes the cut. Between
// two revisions this is O(1); on a revision it walks the blocks once and hands
// back the ones whose contents did not move, which is a per-turn cost, not a
// per-frame one.
function useBlocks(items: Item[], cut: number, revision: number): Item[][] {
  const held = useRef<{ at: number; cut: number; blocks: Item[][] }>({ at: -1, cut: -1, blocks: [] });
  if (held.current.at === revision && held.current.cut === cut) return held.current.blocks;

  const prev = held.current.blocks;
  const blocks: Item[][] = [];
  for (let at = 0; at < cut; at += BLOCK) {
    const end = Math.min(at + BLOCK, cut);
    const old = prev[blocks.length];
    let same = old !== undefined && old.length === end - at;
    for (let i = 0; same && i < end - at; i++) same = old[i] === items[at + i];
    blocks.push(same ? old : items.slice(at, end));
  }
  held.current = { at: revision, cut, blocks };
  return blocks;
}

export function Transcript({ items, revision, waiting, scroll, hidden, onPinned, jump, onApprove, onAnswer, onSuggest, onForget, onExtInvoke, onExtSubmit, takeovers = {}, checkpoints, onPrepareRewind, onCommitRewind, onUndoRewind }: Props) {
  // A block the selection touches must not leave the DOM. Unmounting the node a
  // selection is anchored to makes the browser remap that selection onto
  // whatever is still mounted — which reads as "I selected up there and the
  // bottom got selected too".
  const [held, setHeld] = useState<Set<number>>(new Set());

  // Stick to the bottom only while the reader is already there; scrolling up
  // must not be yanked back by incoming frames.
  const [pinned, setPinned] = useState(true);
  const end = useRef<HTMLDivElement>(null);
  const flow = useRef<HTMLDivElement>(null);
  // Read from observer callbacks that must not be torn down and rebuilt every
  // time the reader crosses the bottom.
  const at = useRef(pinned);
  at.current = pinned;

  // Set while a follow is scrolling, so the resize it causes is not read as new
  // content arriving.
  const self = useRef(false);
  // When the reader last touched an input device. Coming back to the bottom
  // only resumes the follow if they are the one who went there.
  const gesture = useRef(0);
  const wasAt = useRef(0);
  // Scroll events before this moment are the browser's, not the reader's.
  const quiet = useRef(0);

  // Scrolls inside the layout pass, not on the next frame. Deferring it left a
  // window where the DOM had grown and the scroller had not moved yet, and the
  // reveal clock — which appends characters on its own rAF — kept reopening it:
  // two loops chasing each other, measured as the distance to the bottom
  // swinging 0→142px through the whole stream. That swing is the jitter.
  const follow = useCallback(() => {
    self.current = true;
    end.current?.scrollIntoView({ block: "end" });
    requestAnimationFrame(() => {
      self.current = false;
    });
  }, []);

  useEffect(() => {
    const onSelect = () => {
      const sel = document.getSelection();
      const root = scroll.current;
      if (!root || !sel || sel.isCollapsed || sel.rangeCount === 0) {
        setHeld((prev) => (prev.size ? new Set() : prev));
        return;
      }
      const range = sel.getRangeAt(0);
      if (!root.contains(range.commonAncestorContainer)) return;
      const next = new Set<number>();
      root.querySelectorAll(".chunk").forEach((el, i) => {
        if (range.intersectsNode(el)) next.add(i);
      });
      setHeld((prev) => (prev.size === next.size && [...next].every((i) => prev.has(i)) ? prev : next));
    };
    document.addEventListener("selectionchange", onSelect);
    return () => document.removeEventListener("selectionchange", onSelect);
  }, [scroll]);

  // Following is about intent. Two things carry it, and neither alone is
  // enough. The input devices are unambiguous — a wheel, a key, a drag on the
  // scrollbar is the reader, always — and they release immediately, without
  // waiting for a position test that virtualisation makes unreliable. But a
  // scroll can also arrive with no gesture at all (a screen reader moving the
  // cursor, the browser's own find), and for those the position is the only
  // evidence there is: an upward move that leaves the bottom behind is the
  // reader, one that stays inside the margin is the transcript growing.
  // Zero, not now: this stamp is what lets the bottom marker resume the follow,
  // and a gesture that just left the bottom must not also license going back to
  // it. Only a downward one does.
  const release = useCallback(() => {
    gesture.current = 0;
    if (!at.current) return;
    at.current = false;
    setPinned(false);
    onPinned(false);
  }, [onPinned]);

  useEffect(() => {
    const root = scroll.current;
    if (!root) return;
    const mark = () => {
      gesture.current = performance.now();
    };
    const onWheel = (e: WheelEvent) => (e.deltaY < 0 ? release() : mark());
    const onKey = (e: KeyboardEvent) => {
      if (["ArrowUp", "PageUp", "Home"].includes(e.key)) release();
      else if (["ArrowDown", "PageDown", "End", " "].includes(e.key)) mark();
    };
    // Reading scrollTop is free; scrollHeight and clientHeight force a layout,
    // which is why they are read only here — while following, on an upward
    // event — and never per frame.
    const onScroll = () => {
      const top = root.scrollTop;
      const up = top < wasAt.current - 2;
      wasAt.current = top;
      if (!up || self.current || !at.current || performance.now() < quiet.current) return;
      if (root.scrollHeight - top - root.clientHeight <= 48) return;
      release();
    };
    root.addEventListener("wheel", onWheel, { passive: true });
    root.addEventListener("keydown", onKey);
    root.addEventListener("pointerdown", mark);
    root.addEventListener("touchmove", mark, { passive: true });
    root.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      root.removeEventListener("wheel", onWheel);
      root.removeEventListener("keydown", onKey);
      root.removeEventListener("pointerdown", mark);
      root.removeEventListener("touchmove", mark);
      root.removeEventListener("scroll", onScroll);
    };
  }, [scroll, onPinned, release]);

  // One observer for every block, not one per block. Hundreds of separate
  // observers did not reliably report a block leaving — blocks stayed mounted
  // 400,000px above the viewport — and each one is per-frame work of its own.
  // A single observer watching many targets is what the API is for.
  const watchers = useRef(new Map<Element, (near: boolean) => void>());
  const lens = useRef<IntersectionObserver | null>(null);

  // Not while hidden: nothing intersects a display:none root, so the observer
  // would report every block as far away and unmount the entire transcript.
  // Coming back would then have to rebuild it from estimated heights under a
  // scroll position that no longer means anything. A pane on another tab keeps
  // what it had — that is the whole point of leaving it mounted.
  useEffect(() => {
    const root = scroll.current;
    if (!root || hidden) return;
    // Two screens of margin: a block is mounted well before it can be scrolled
    // to, so reaching it never waits on a render.
    const obs = new IntersectionObserver(
      (entries) => {
        for (const e of entries) watchers.current.get(e.target)?.(e.isIntersecting);
      },
      { root, rootMargin: "200% 0px" },
    );
    lens.current = obs;
    for (const el of watchers.current.keys()) obs.observe(el);
    return () => {
      obs.disconnect();
      lens.current = null;
    };
  }, [scroll, hidden]);

  // Stable, so a block subscribes once for its lifetime.
  const watch = useCallback((el: Element, cb: (near: boolean) => void) => {
    watchers.current.set(el, cb);
    lens.current?.observe(el);
    return () => {
      watchers.current.delete(el);
      lens.current?.unobserve(el);
    };
  }, []);

  // Coming back to the bottom resumes it. Watching a marker at the end of the
  // flow answers that without measuring the scroller, which following a live
  // answer would otherwise do on every frame.
  useEffect(() => {
    const marker = end.current;
    const root = scroll.current;
    if (!marker || !root || hidden) return;
    const io = new IntersectionObserver(
      ([e]) => {
        if (!e.isIntersecting) return;
        // The marker crossing back into view is not by itself a reason to
        // resume: blocks mounting and unmounting move it past this edge on
        // their own, and every one of those was yanking the reader back down.
        // Only a hand on the wheel counts, and only just now.
        if (performance.now() - gesture.current > 400 && performance.now() > quiet.current) return;
        at.current = true;
        setPinned(true);
        onPinned(true);
      },
      { root, rootMargin: "0px 0px 48px 0px" },
    );
    io.observe(marker);
    return () => io.disconnect();
  }, [scroll, hidden, onPinned]);

  // A hidden container scrolls nothing: scrollIntoView is a no-op there, and
  // every block unmounts because nothing intersects a display:none root. So
  // coming back has to do two things. Restore the follow — otherwise the
  // transcript sits where you left it, thousands of pixels above what arrived
  // meanwhile. And ignore the scroll events of the next few frames: putting
  // scrollTop back and remounting the blocks both move it, and reading either
  // as "the reader scrolled up" is what silently ended the follow.
  useEffect(() => {
    if (hidden) return;
    quiet.current = performance.now() + 400;
    if (at.current) follow();
  }, [hidden, follow]);

  // Before paint: the reader must never see a frame where new text landed and
  // the view had not caught up.
  useLayoutEffect(() => {
    if (pinned) follow();
  }, [items, pinned, follow]);

  // A mark's message may be inside an unmounted block, which has no node to
  // scroll to. Land on the block first — that always exists, holding its own
  // height — and the observer mounts it because it is now in range; the second
  // pass then puts the message itself under the reader. Marked as a gesture,
  // or the follow would read this scroll as the transcript moving and stay
  // pinned to the bottom the reader just left.
  const jumpTo = useCallback(
    (mark: RailMark) => {
      const root = scroll.current;
      const inner = flow.current;
      if (!root || !inner) return;
      gesture.current = 0;
      at.current = false;
      setPinned(false);
      onPinned(false);
      // Measured against the scroller, not offsetTop: the nearest positioned
      // ancestor is .pane, which does not scroll, so offsetTop answers "where
      // in the window" — writing that back as scrollTop barely moves anything.
      const topOf = (el: HTMLElement) => el.getBoundingClientRect().top - root.getBoundingClientRect().top + root.scrollTop;
      const chunk = inner.querySelectorAll<HTMLElement>(".chunk")[mark.block];
      if (chunk) root.scrollTop = topOf(chunk) + (mark.of > 1 ? (mark.within / mark.of) * chunk.offsetHeight : 0) - 12;
      const settle = (tries: number) => {
        const el = inner.querySelector<HTMLElement>(`[data-item="${CSS.escape(mark.id)}"]`);
        if (el) {
          root.scrollTop = topOf(el) - 12;
          el.setAttribute("data-hit", "");
          setTimeout(() => el.removeAttribute("data-hit"), 1200);
          return;
        }
        if (tries > 0) requestAnimationFrame(() => settle(tries - 1));
      };
      requestAnimationFrame(() => settle(6));
    },
    [scroll, flow, onPinned],
  );

  // "Back to latest" cannot just write scrollTop once: where the bottom is
  // changes as blocks mount under it. Turning the follow back on lets the same
  // correction that tracks a live answer carry it the rest of the way.
  useEffect(() => {
    if (!jump) return;
    at.current = true;
    setPinned(true);
    onPinned(true);
    follow();
  }, [jump, follow, onPinned]);

  // Blocks mounting and unmounting move the bottom out from under a follow that
  // only fires on events — which left the viewport stranded half a transcript
  // up. Watching the height itself covers every cause: a block loading, a card
  // expanding, a webfont finally arriving.
  useEffect(() => {
    const el = flow.current;
    if (!el) return;
    const ro = new ResizeObserver(() => {
      if (at.current) follow();
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, [follow]);

  // Only a message still being written can grow; anything else is final the
  // moment it lands, so the split is exactly one card wide.
  const last = items.length > 0 ? items[items.length - 1] : undefined;
  const live = last?.t === "say" && !last.done ? last : undefined;
  const blocks = useBlocks(items, live ? items.length - 1 : items.length, revision);

  const rowProps = { onApprove, onAnswer, onForget, onExtInvoke, takeovers, onExtSubmit, onPrepareRewind, onCommitRewind, onUndoRewind };

  // What you said, and where it sits. Derived from the same blocks the
  // transcript renders, so a mark always knows which block holds it — that is
  // what makes it locatable while the message itself is unmounted.
  const marks: RailMark[] = [];
  blocks.forEach((block, b) =>
    block.forEach((it, i) => {
      if (it.t !== "user") return;
      marks.push({
        id: it.id,
        text: it.text,
        at: b * BLOCK + i,
        block: b,
        within: i,
        of: block.length,
        files: checkpoints.get(it.id)?.files ?? 0,
      });
    }),
  );

  return (
    <div
      className="scroll"
      id="flowScroll"
      data-pane="flow"
      ref={scroll}
      hidden={hidden}
    >
      <Rail marks={marks} total={items.length} scroll={scroll} flow={flow} onJump={jumpTo} onGrab={release} bound={!hidden} />
      <div className="flow" ref={flow}>
        {items.length === 0 && <Hero onPick={onSuggest} />}
        {blocks.map((block, i) => (
          <Block
            key={i}
            items={block}
            checkpoints={checkpoints}
            watch={watch}
            // The block a new card lands in has no measured height yet, so it
            // mounts without waiting for the observer's first callback.
            eager={i === blocks.length - 1}
            keep={held.has(i)}
            {...rowProps}
          />
        ))}
        {live && <Row it={live} {...rowProps} cp={checkpoints.get(live.id)} />}
        {waiting.ttftSince && <Await retry={waiting.retry} />}
        <div ref={end} className="flow-end" aria-hidden="true" />
      </div>
    </div>
  );
}

// Off-screen cards leave the DOM entirely, holding their place with the height
// they last measured. content-visibility already spared them layout and paint;
// this is about the cost of existing at all — at 4000 turns the nodes alone put
// 300k of them and 111MB behind a window nobody can see through.
const Block = memo(function Block({
  items,
  checkpoints,
  watch,
  eager,
  keep,
  ...rowProps
}: {
  items: Item[];
  checkpoints: Map<string, Checkpoint>;
  watch: (el: Element, cb: (near: boolean) => void) => () => void;
  eager: boolean;
  // The selection reaches into this block, so it stays mounted however far
  // off-screen it scrolls.
  keep: boolean;
} & RowHandlers) {
  const box = useRef<HTMLDivElement>(null);
  const [near, setNear] = useState(eager);
  const tall = useRef(0);

  useEffect(() => (box.current ? watch(box.current, setNear) : undefined), [watch]);

  // Measured while mounted so the placeholder that replaces it is exactly as
  // tall — otherwise unmounting above the viewport would jerk the scroll.
  useLayoutEffect(() => {
    if (near && box.current) tall.current = box.current.offsetHeight;
  });

  return (
    <div
      className="chunk"
      ref={box}
      style={near || keep ? undefined : { height: `${tall.current || items.length * 96}px` }}
    >
      {(near || keep) && items.map((it) => <Row key={it.id} it={it} {...rowProps} cp={checkpoints.get(it.id)} />)}
    </div>
  );
});

interface RowHandlers {
  onApprove: Props["onApprove"];
  onAnswer: Props["onAnswer"];
  onForget: Props["onForget"];
  onExtInvoke: Props["onExtInvoke"];
  takeovers: Record<string, ExtensionSurface>;
  onExtSubmit: Props["onExtSubmit"];
  onPrepareRewind: Props["onPrepareRewind"];
  onCommitRewind: Props["onCommitRewind"];
  onUndoRewind: Props["onUndoRewind"];
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
  takeovers,
  onExtSubmit,
  cp,
  onPrepareRewind,
  onCommitRewind,
  onUndoRewind,
}: RowHandlers & { it: Item; cp?: Checkpoint }) {
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
        <ToolCard
            tool={it.tool}
            running={it.running}
            children={it.children}
            takeover={it.tool.id ? takeovers[`tool:${it.tool.id}`] : undefined}
            onExtInvoke={onExtInvoke}
          />
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
          ? t("连接在响应头前断了，重试 {attempt}/{max} · {secs}s", { attempt: retry.attempt, max: retry.max, secs: secs.toFixed(1) })
          : t("等待回包 {secs}s", { secs: secs.toFixed(1) })}
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
      <div className="t">{t("交待一件事，它自己往下做")}</div>
      <div className="s">
        {t("读代码、联网查证、派子代理、改文件 —— 每一步都同时落进「轨迹」，那是机器记录，不是给人读的叙事。")}
      </div>
      <div className="qs">
        {SUGGESTIONS.map((s) => (
          <button className="sug" key={s} onClick={() => onPick(t(s))}>
            {t(s)}
          </button>
        ))}
      </div>
    </div>
  );
}
