import { useState } from "react";
import { Sym } from "../Sym";
import type { Item } from "../../state/session";

interface Props {
  item: Extract<Item, { t: "ask" }>;
  onAnswer: (itemId: string, id: string, answers: { questionId: string; selected: string[] }[]) => void;
}

export function AskCard({ item, onAnswer }: Props) {
  const qs = item.ask.questions;
  const [tab, setTab] = useState(0);
  const [picks, setPicks] = useState<string[][]>(() => qs.map(() => []));
  // The wire carries selections as free strings and the kernel joins them into
  // the tool result untouched, so "其他" is a label like any other.
  const [other, setOther] = useState<string[]>(() => qs.map(() => ""));
  const [otherOn, setOtherOn] = useState<boolean[]>(() => qs.map(() => false));
  // Answered is read-only but still readable: the tabs keep working so you can
  // see what was chosen for each question, and the options stay on screen with
  // the unchosen ones dimmed by the sealed styling.
  const sealed = item.answered !== undefined;
  const chosen = item.answered ?? picks;

  const free = (i: number) => (otherOn[i] ? other[i].trim() : "");
  const selected = (i: number) => (free(i) ? [...picks[i], free(i)] : picks[i]);
  const answered = (i: number) => selected(i).length > 0;
  const left = qs.reduce((n, _, i) => n + (answered(i) ? 0 : 1), 0);
  // A sealed card may have been answered by another client, so what counts as
  // free text is whatever came back that no option offered.
  const sealedFree = (i: number) => {
    const offered = new Set(qs[i].options.map((o) => o.label));
    return (chosen[i] ?? []).filter((v) => !offered.has(v)).join("、");
  };
  const freeShown = (i: number) => (sealed ? sealedFree(i) : other[i]);

  const at = <T,>(list: T[], i: number, v: T) => list.map((x, k) => (k === i ? v : x));

  const toggle = (qi: number, label: string) => {
    if (sealed) return;
    setPicks((prev) => {
      if (!qs[qi].multi) return at(prev, qi, [label]);
      const has = prev[qi].includes(label);
      return at(prev, qi, has ? prev[qi].filter((l) => l !== label) : [...prev[qi], label]);
    });
    if (!qs[qi].multi) setOtherOn((prev) => at(prev, qi, false));
  };

  const toggleOther = (qi: number) => {
    if (sealed) return;
    const on = !otherOn[qi];
    setOtherOn((prev) => at(prev, qi, on));
    if (on && !qs[qi].multi) setPicks((prev) => at(prev, qi, []));
  };

  const send = (answers: string[][]) =>
    onAnswer(item.id, item.ask.id, qs.map((q, i) => ({ questionId: q.id, selected: answers[i] })));

  return (
    <div className="call" data-k="ask">
      <div className="g">
        <Sym glyph="?" />
        <span className="line" />
      </div>
      <div className="c">
        <div className="hl">
          <span className="nm">Ask</span>
          <span className="arg">{qs.length} 个问题</span>
        </div>
        <div className="out">
          <div className="ask" data-sealed={sealed ? "" : undefined}>
            {qs.length > 1 && (
              <div className="ask-tabs" role="tablist">
                {qs.map((q, i) => (
                  <button
                    key={q.id}
                    className="ask-tab"
                    role="tab"
                    aria-selected={i === tab}
                    data-answered={answered(i) ? "" : undefined}
                    onClick={() => setTab(i)}
                  >
                    {q.header || `问题 ${i + 1}`}
                    <i className="dot" />
                  </button>
                ))}
              </div>
            )}
            {qs.map((q, i) => (
              <div className="ask-pane" key={q.id} data-on={i === tab ? "" : undefined}>
                <div className="ask-q">{q.prompt}</div>
                <div className="ask-hint">{q.multi ? "可多选" : "选一个"}</div>
                <div className="opts">
                  {q.options.map((o, j) => (
                    <button
                      key={o.label}
                      className="opt"
                      data-multi={q.multi ? "" : undefined}
                      data-on={chosen[i]?.includes(o.label) ? "" : undefined}
                      onClick={() => toggle(i, o.label)}
                    >
                      <span className="mark" />
                      <span className="txt">
                        <span className="lb">
                          {o.label}
                          {j === 0 && !q.multi && <span className="rec">推荐</span>}
                        </span>
                        {o.description && <span className="ds">{o.description}</span>}
                      </span>
                    </button>
                  ))}
                  <button
                    className="opt opt-other"
                    data-multi={q.multi ? "" : undefined}
                    data-on={(sealed ? !!sealedFree(i) : otherOn[i]) ? "" : undefined}
                    onClick={() => toggleOther(i)}
                  >
                    <span className="mark" />
                    <span className="txt">
                      <span className="lb">其他 —— 我自己写</span>
                    </span>
                  </button>
                </div>
                <div className="other-wrap" data-on={(sealed ? !!sealedFree(i) : otherOn[i]) ? "" : undefined}>
                  <input
                    value={freeShown(i)}
                    readOnly={sealed}
                    placeholder="你想要的第几种做法，写在这儿"
                    onChange={(e) => setOther((prev) => at(prev, i, e.target.value))}
                  />
                </div>
              </div>
            ))}
            {sealed && (
              <div className="ask-done">
                {qs.map((q, i) => (
                  <span key={q.id}>
                    {i > 0 && "　·　"}
                    <b>{q.header || `问题 ${i + 1}`}：</b>
                    {chosen[i]?.length ? chosen[i].join("、") : "未答"}
                  </span>
                ))}
              </div>
            )}
            {!sealed && (
              <div className="ask-foot">
                <button className="btn" data-primary disabled={left > 0} onClick={() => send(qs.map((_, i) => selected(i)))}>
                  {left ? `确认（还有 ${left} 个没答）` : "确认"}
                </button>
                {/* An answer batch with nothing selected is the kernel's explicit
                    "don't decide for me" path: it ends the turn rather than
                    feeding a prose dismissal back to the model. */}
                <button className="dismiss" onClick={() => send(qs.map(() => []))}>
                  先不选择，直接回复
                </button>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
