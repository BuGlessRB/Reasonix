import type { McpEntry } from "../../port/port";
import { t } from "../../i18n";

// Only the servers that need a decision get a row. A healthy MCP fleet is the
// least interesting thing on this rail: it is all green and never changes, and
// where its tools came from is already stamped on the tool card that used them.
// The row is a button because reading "connect failed" and being able to do
// something about it should not be two separate discoveries.
export function Mcp({ servers, onOpen }: { servers: McpEntry[]; onOpen: () => void }) {
  const broken = servers.filter((s) => s.state === "failed");
  if (broken.length === 0) return null;

  return (
    <div className="block" data-b="mcp">
      <div className="lbl">
        {t("外部服务")}<span className="c">{t("{n} 个连不上", { n: broken.length })}</span>
      </div>
      <div className="jobs">
        {broken.map((s) => (
          <button className="job" key={s.name} data-done="" onClick={onOpen} title={t("到设置里重连")}>
            <i className="pip" style={{ background: "var(--err)" }} />
            <span className="cmd">{s.name}</span>
            <span className="rt" title={s.error}>
              {s.error || t("失败")}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}
