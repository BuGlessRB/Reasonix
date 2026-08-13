import { useEffect, useRef, useState } from "react";
import type { Item, Waiting } from "../state/session";
import type { ApprovalVerdict } from "../port/port";
import { RMark } from "./RMark";
import { ToolCard } from "./cards/ToolCard";
import { GuardianCard } from "./cards/GuardianCard";
import { ApprovalCard } from "./cards/ApprovalCard";
import { AskCard } from "./cards/AskCard";
import { SayCard } from "./cards/SayCard";
import { AskFromTool } from "./cards/AskFromTool";

interface Props {
  items: Item[];
  waiting: Waiting;
  onApprove: (id: string, v: ApprovalVerdict) => void;
  onAnswer: (id: string, answers: { questionId: string; selected: string[] }[]) => void;
}

export function Transcript({ items, waiting, onApprove, onAnswer }: Props) {
  const scroll = useRef<HTMLDivElement>(null);
  // Stick to the bottom only while the reader is already there; scrolling up
  // must not be yanked back by incoming frames.
  const [pinned, setPinned] = useState(true);

  useEffect(() => {
    if (!pinned) return;
    const el = scroll.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [items, pinned]);

  return (
    <div
      className="scroll"
      ref={scroll}
      onScroll={(e) => {
        const el = e.currentTarget;
        setPinned(el.scrollHeight - el.scrollTop - el.clientHeight < 48);
      }}
    >
      <div className="flow">
        {items.length === 0 && <Hero />}
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
              return it.tool.name === "ask" ? (
                <AskFromTool key={it.id} tool={it.tool} onAnswer={onAnswer} />
              ) : (
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

function Hero() {
  return (
    <div className="hero">
      <RMark />
      <div className="t">交待一件事，它自己往下做</div>
      <div className="s">
        读代码、联网查证、派子代理、改文件 —— 每一步都同时落进「轨迹」，那是机器记录，不是给人读的叙事。
      </div>
    </div>
  );
}
