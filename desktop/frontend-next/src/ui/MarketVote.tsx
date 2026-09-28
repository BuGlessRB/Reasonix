import { useEffect, useState } from "react";
import { t } from "../i18n";
import { ACCOUNT_SIGNIN_DISABLED, reason } from "../i18n/kernel";
import { HttpError, type AgentPort, type MarketPackage, type MarketVote as Vote } from "../port/port";

// How the list reads a package's reception. The share is the registry's
// up / (up + down); with no votes there is no share, which is not 0%.
export function approvalLabel(p: Pick<MarketPackage, "approvalRate" | "upCount" | "downCount">): string {
  if (p.approvalRate == null) return t("暂无评价");
  return t("好评 {pct}%（{n} 票）", { pct: Math.round(p.approvalRate * 100), n: p.upCount + p.downCount });
}

// One vote per account: pressing the lit button again withdraws it. Whether
// this account may vote is the registry's answer, never guessed here.
export function MarketVote({ port, pkg, onSignIn }: { port: AgentPort; pkg: MarketPackage; onSignIn?: () => void }) {
  const [vote, setVote] = useState<Vote | null>(null);
  const [off, setOff] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let live = true;
    port
      .marketMyVote(pkg.slug)
      .then((v) => live && setVote(v))
      .catch((e) => {
        if (!live) return;
        // A paired device cannot spend this machine's account token: show the
        // tally alone rather than buttons that would always refuse.
        if (e instanceof HttpError && e.reason?.code === ACCOUNT_SIGNIN_DISABLED) setOff(true);
        else setError(reason(e));
        setVote({ signedIn: false, value: 0 });
      });
    return () => {
      live = false;
    };
  }, [port, pkg.slug]);

  const up = vote?.upCount ?? pkg.upCount;
  const down = vote?.downCount ?? pkg.downCount;
  const rate = vote?.approvalRate !== undefined ? vote.approvalRate : pkg.approvalRate;
  const mine = vote?.value ?? 0;
  const signedIn = !!vote?.signedIn;
  const can = signedIn && vote?.canVote !== false;

  const cast = async (value: -1 | 1) => {
    setBusy(true);
    setError("");
    try {
      setVote(await port.voteMarket(pkg.slug, mine === value ? 0 : value));
    } catch (e) {
      setError(reason(e));
    } finally {
      setBusy(false);
    }
  };

  const note = !vote
    ? ""
    : off
      ? t("在这台设备上不能评价")
      : !signedIn
        ? ""
        : vote.own
          ? t("不能评价自己发布的包")
          : vote.emailVerified === false
            ? t("验证账号邮箱后才能评价")
            : "";

  return (
    <div className="mkt-vote" data-mine={mine}>
      <span className="mkt-vote-rate">{approvalLabel({ approvalRate: rate ?? null, upCount: up, downCount: down })}</span>
      <div className="mkt-vote-btns" role="group" aria-label={t("评价")}>
        <button
          className="act"
          data-action="market.vote"
          data-value="up"
          aria-pressed={mine === 1}
          disabled={!can || busy}
          title={mine === 1 ? t("撤回赞") : t("赞")}
          onClick={() => void cast(1)}
        >
          <span aria-hidden="true">▲</span> {t("赞")} <span className="n">{up}</span>
        </button>
        <button
          className="act"
          data-action="market.vote"
          data-value="down"
          aria-pressed={mine === -1}
          disabled={!can || busy}
          title={mine === -1 ? t("撤回踩") : t("踩")}
          onClick={() => void cast(-1)}
        >
          <span aria-hidden="true">▼</span> {t("踩")} <span className="n">{down}</span>
        </button>
      </div>
      {vote && !signedIn && !off && (
        <button className="act mkt-vote-signin" data-action="market.signin" onClick={onSignIn} disabled={!onSignIn}>
          {t("登录后评价")}
        </button>
      )}
      {note && <span className="note">{note}</span>}
      {error && <span className="why">{error}</span>}
    </div>
  );
}
