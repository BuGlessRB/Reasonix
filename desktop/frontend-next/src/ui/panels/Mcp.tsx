import type { McpEntry } from "../../port/port";
import { t } from "../../i18n";

// Only the servers that need a decision get a row. A healthy MCP fleet is the
// least interesting thing on this rail: it is all green and never changes, and
// where its tools came from is already stamped on the tool card that used them.
// The row is a button because reading "connect failed" and being able to do
// something about it should not be two separate discoveries.

// The rail reports, it does not enumerate — the same cap the delegate and file
// panels take, at the count two-line rows stop fitting beside everything else.
const SHOWN = 3;

export function Mcp({ servers, onOpen }: { servers: McpEntry[]; onOpen: () => void }) {
  const broken = servers.filter((s) => s.state === "failed");
  if (broken.length === 0) return null;
  const shown = broken.slice(0, SHOWN);

  return (
    <div className="block" data-b="mcp">
      <div className="lbl">
        {t("外部服务")}<span className="c">{t("{n} 个连不上", { n: broken.length })}</span>
      </div>
      <div className="srvs">
        {shown.map((s) => (
          <button className="srvrow" key={s.name} onClick={onOpen} title={t("到设置里重连")}>
            <span className="hd">
              <i className="pip" />
              <span className="nm">{s.name}</span>
            </span>
            {/* The endpoint's own words, so its length is not ours to decide.
                Clamped here and whole in the tooltip. */}
            <span className="why" title={s.error}>{s.error || t("失败")}</span>
          </button>
        ))}
        {broken.length > shown.length && (
          <span className="more">{t("还有 {n} 个，都在设置里", { n: broken.length - shown.length })}</span>
        )}
      </div>
    </div>
  );
}
