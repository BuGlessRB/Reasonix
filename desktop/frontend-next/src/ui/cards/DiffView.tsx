import { t } from "../../i18n";
export function DiffView({ diff, path }: { diff: string; path?: string }) {
  const lines = diff.split("\n").filter((l) => l.length > 0);
  return (
    <div className="dif">
      <div className="dif-hd">
        <span>{path ?? t("改动")}</span>
        <span className="ro">{t("只读")}</span>
      </div>
      {lines.map((l, i) => {
        const sign = l[0] === "+" || l[0] === "-" ? l[0] : " ";
        return (
          <div className="dl" key={i} data-d={sign === " " ? undefined : sign}>
            <span className="no">{i + 1}</span>
            <span className="sg">{sign}</span>
            <span className="cd">{l.slice(sign === " " ? 0 : 1)}</span>
          </div>
        );
      })}
    </div>
  );
}
