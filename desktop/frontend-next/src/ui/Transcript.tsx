import { useEffect, useState, type RefObject } from "react";
import type { Item, Waiting } from "../state/session";
import type { ApprovalVerdict } from "../port/port";
import { RMark } from "./RMark";
import { ToolCard } from "./cards/ToolCard";
import { GuardianCard } from "./cards/GuardianCard";
import { ApprovalCard } from "./cards/ApprovalCard";
import { AskCard } from "./cards/AskCard";
import { SayCard } from "./cards/SayCard";

interface Props {
  items: Item[];
  waiting: Waiting;
  scroll: RefObject<HTMLDivElement | null>;
  hidden: boolean;
  onPinned: (v: boolean) => void;
  onApprove: (itemId: string, id: string, v: ApprovalVerdict) => void;
  onAnswer: (itemId: string, id: string, answers: { questionId: string; selected: string[] }[]) => void;
  onSuggest: (text: string) => void;
}

export function Transcript({ items, waiting, scroll, hidden, onPinned, onApprove, onAnswer, onSuggest }: Props) {
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
        {items.map((it) => {
          switch (it.t) {
            case "user":
              return (
                <div className="call" data-k="me" key={it.id} data-pending={it.pending ? "" : undefined}>
                  <div className="g">
                    <span className="sym">你</span>
                    <span className="line" />
                  </div>
                  <div className="c">
                    <div className="hl">
                      <span className="nm">我</span>
                      {it.pending && <span className="pend">排队中 · 下一个工具边界送达</span>}
                    </div>
                    <div className="out">
                      <div className="txt">{it.text}</div>
                    </div>
                  </div>
                </div>
              );
            case "say":
              // A turn that only produced reasoning is the preamble to the tool
              // call right after it, not a message of its own.
              return it.text.trim() ? <SayCard key={it.id} item={it} /> : null;
            case "tool":
              // The ask tool also raises ask_request, which carries the id
              // /answer needs. Drawing the tool call too put two copies of the
              // same question on screen, each answerable.
              return it.tool.name === "ask" ? null : (
                <ToolCard key={it.id} tool={it.tool} running={it.running} children={it.children} />
              );
            case "guardian":
              return <GuardianCard key={it.id} g={it.g} />;
            case "approval":
              return <ApprovalCard key={it.id} item={it} onApprove={onApprove} />;
            case "ask":
              return <AskCard key={it.id} item={it} onAnswer={onAnswer} />;
            case "compaction":
              return (
                <div className="call" key={it.id}>
                  <div className="g">
                    <span className="sym">⊘</span>
                    <span className="line" />
                  </div>
                  <div className="c">
                    <div className="hl">
                      <span className="nm">{it.done ? "压缩完成" : "正在压缩…"}</span>
                    </div>
                    {it.c.summary && (
                      <div className="out">
                        <div className="txt">{it.c.summary}</div>
                      </div>
                    )}
                  </div>
                </div>
              );
            case "notice":
              return (
                <div className="call" key={it.id}>
                  <div className="g">
                    <span className="sym">·</span>
                    <span className="line" />
                  </div>
                  <div className="c">
                    <div className="out">
                      <div className="txt">{it.text}</div>
                    </div>
                  </div>
                </div>
              );
          }
        })}
        {waiting.ttftSince && <Await retry={waiting.retry} />}
      </div>
    </div>
  );
}

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
