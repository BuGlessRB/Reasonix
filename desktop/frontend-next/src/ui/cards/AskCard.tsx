import { useState } from "react";
import type { Item } from "../../state/session";

interface Props {
  item: Extract<Item, { t: "ask" }>;
  onAnswer: (itemId: string, id: string, answers: { questionId: string; selected: string[] }[]) => void;
}

export function AskCard({ item, onAnswer }: Props) {
  const qs = item.ask.questions;
  const [tab, setTab] = useState(0);
  const [picks, setPicks] = useState<string[][]>(() => qs.map(() => []));
  const answered = (i: number) => picks[i].length > 0;
  const left = qs.reduce((n, _, i) => n + (answered(i) ? 0 : 1), 0);
  // Answered is read-only but still readable: the tabs keep working so you can
  // see what was chosen for each question, and the options stay on screen with
  // the unchosen ones dimmed by the sealed styling.
  const sealed = item.answered !== undefined;
  const chosen = item.answered ?? picks;

  const toggle = (qi: number, label: string) => {
    setPicks((prev) => {
      const next = prev.map((p) => [...p]);
      const multi = qs[qi].multi;
      if (!multi) next[qi] = [label];
      else if (next[qi].includes(label)) next[qi] = next[qi].filter((l) => l !== label);
      else next[qi].push(label);
      return next;
    });
  };

  return (
    <div className="call" data-k="ask">
      <div className="g">
        <span className="sym">?</span>
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
                      data-on={picks[i].includes(o.label) ? "" : undefined}
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
              <button
                className="btn"
                data-primary
                disabled={left > 0}
                onClick={() => onAnswer(item.id, item.ask.id, qs.map((q, i) => ({ questionId: q.id, selected: picks[i] })))}
              >
                {left ? `确认（还有 ${left} 个没答）` : "确认"}
              </button>
            </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
