import type { Item } from "../../state/session";
import { Sym } from "../Sym";

// A notice carries a level and the card ignored it, so a warning read exactly
// like a status line. Severity is a left colour bar everywhere else in this UI
// (.guard, .find) — a notice speaks the same language rather than a new one.
//
// The detail is the second half of the same sentence: the kernel writes the
// headline for a person and the diagnostic underneath it, and a card that
// concatenated them shipped one unreadable line of bold text. .why is where
// every other card in this UI puts the part you read second.
export function NoticeCard({ item }: { item: Extract<Item, { t: "notice" }> }) {
  // A failure and a caution read the same when both come out amber, and the
  // one worth stopping for is the failure. The sheet already has both rails.
  const lvl = item.level === "error" ? "err" : item.level === "warn" ? "warn" : undefined;
  return (
    <div className="call">
      <div className="g">
        <Sym glyph="·" />
        <span className="line" />
      </div>
      <div className="c">
        <div className="out">
          <div className="finds">
            <div className="find" data-lvl={lvl}>
              <span className="t">{item.text}</span>
              {item.detail && <span className="why nwhy">{item.detail}</span>}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
