import { useCallback, useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import type { HubPort } from "../port/hub";
import { copyText } from "./CopyButton";
import { useDismiss } from "./dismiss";
import { clock, deviceLabel, type Share, useShare } from "./PhoneAccess";
import { Switch } from "./Switch";

/** The chrome's way to put a phone on this window: a code on a card, one
 *  press from anywhere. Drawn only where the kernel has a window to share —
 *  a browser tab or a paired phone gets nothing here. */
export function PhonePop({ hub, onError }: { hub: HubPort; onError: (e: unknown) => void }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setOpen(false), []);
  useDismiss(open, box, close);
  const share = useShare(hub, onError, open);
  const { refresh, newCode } = share;
  const shareOpen = share.st?.open ?? false;
  const hasOffer = share.offer !== null;

  // Another surface may have opened or shut the door since this card was last
  // drawn, so opening it reads first.
  useEffect(() => {
    if (open) void refresh().catch(() => {});
  }, [open, refresh]);

  // Opening the card is asking for a code. Once per opening: after a phone
  // spends it, the next one is asked for by hand rather than minted behind it.
  const minted = useRef(false);
  useEffect(() => {
    if (!open) {
      minted.current = false;
      return;
    }
    if (shareOpen && !hasOffer && !minted.current) {
      minted.current = true;
      void newCode();
    }
  }, [open, shareOpen, hasOffer, newCode]);

  if (!share.st) return null;
  const paired = share.st.devices.length;
  return (
    <div className="phonepop" ref={box}>
      <button
        className="thbtn phone-action"
        data-action="share.card"
        data-live={shareOpen ? "" : undefined}
        aria-expanded={open}
        aria-label={t("手机扫码访问")}
        title={shareOpen ? t("手机访问已开启 · {n} 台已连接", { n: paired }) : t("手机扫码访问")}
        onClick={() => setOpen((v) => !v)}
      >
        <svg viewBox="0 0 16 16" aria-hidden="true">
          <rect x="2.5" y="2.5" width="4" height="4" rx=".6" />
          <rect x="9.5" y="2.5" width="4" height="4" rx=".6" />
          <rect x="2.5" y="9.5" width="4" height="4" rx=".6" />
          <path d="M9.5 9.5h1.6v1.6M13.5 9.5v.01M9.5 13.5h.01M12 12h1.5v1.5H12Z" />
        </svg>
      </button>
      {open && <PhoneCard share={share} />}
    </div>
  );
}

/** The card's own arrangement of the share: the switch in the header, the code
 *  as the one large thing, everything else a line. The settings block keeps
 *  the long form, where there is room to explain. */
function PhoneCard({ share }: { share: Share }) {
  const { st, ip, pick, offer, busy, newCode, toggle, revoke } = share;
  const [copied, setCopied] = useState(false);
  const [arming, setArming] = useState("");
  const arm = useRef<number | null>(null);
  useEffect(() => () => { if (arm.current !== null) window.clearTimeout(arm.current); }, []);
  if (!st) return null;
  const noNetwork = st.addresses.length === 0;
  const chosen = ip || (st.open ? st.origin?.replace(/^https?:\/\//, "").replace(/:\d+$/, "") : "") || st.addresses[0]?.ip || "";

  const copy = (url: string) =>
    copyText(url)
      .then(() => {
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1600);
      })
      .catch(() => {});

  // Disconnecting takes a second press on the same place, which is cheaper
  // than a dialog in a card this small and still not one slip.
  const disconnect = (id: string) => {
    if (arm.current !== null) window.clearTimeout(arm.current);
    if (arming !== id) {
      setArming(id);
      arm.current = window.setTimeout(() => setArming(""), 3000);
      return;
    }
    setArming("");
    void revoke(id);
  };

  return (
    <div className="phonecard" role="dialog" aria-label={t("手机扫码访问")}>
      <header>
        <span>
          <b>{t("手机访问")}</b>
          <small>{noNetwork ? t("这台电脑现在没有局域网地址") : t("同一网络里的手机扫码即可操作这里的会话")}</small>
        </span>
        <Switch data-action="share.toggle" on={st.open} busy={busy || noNetwork} label={t("允许手机访问")} onClick={() => void toggle()} />
      </header>

      {st.open && (
        <div className="pc-code">
          {offer ? (
            <>
              <img src={`data:image/svg+xml;charset=utf-8,${encodeURIComponent(offer.qr)}`} alt={t("配对二维码")} width={148} height={148} />
              <span className="pc-when">{t("扫码配对 · {time} 前有效", { time: clock(offer.expires) })}</span>
              <span className="pc-acts">
                <button data-action="share.copy" onClick={() => void copy(offer.url)}>{copied ? t("已复制") : t("复制链接")}</button>
                <i aria-hidden="true" />
                <button data-action="share.offer" disabled={busy} onClick={() => void newCode()}>{t("换一个")}</button>
              </span>
            </>
          ) : (
            <button className="pc-mint" data-action="share.offer" disabled={busy} onClick={() => void newCode()}>
              {t("显示配对二维码")}
            </button>
          )}
        </div>
      )}

      {st.addresses.length > 1 && (
        <label className="pc-row">
          <span>{t("网络")}</span>
          <select data-action="share.address" value={chosen} disabled={busy} onChange={(e) => pick(e.target.value)}>
            {st.addresses.map((a) => (
              <option key={a.ip} value={a.ip}>
                {a.kind === "tailnet" ? `${a.ip} · Tailscale` : a.kind === "virtual" ? `${a.ip} · ${t("虚拟网卡")}` : `${a.ip} · ${a.interface}`}
              </option>
            ))}
          </select>
        </label>
      )}

      {st.open && (
        <section>
          <div className="pc-hd">
            <b>{t("已连接")}</b>
            <small>{st.devices.length}</small>
          </div>
          {st.devices.map((d, i) => (
            <div className="pc-dev" key={d.id}>
              <i aria-hidden="true" />
              <span title={d.name}>{deviceLabel(i)}</span>
              <small>{clock(d.lastSeen)}</small>
              <button
                data-action={arming === d.id ? "share.revoke" : "share.ask-revoke"}
                data-target={d.id}
                data-armed={arming === d.id ? "" : undefined}
                onClick={() => disconnect(d.id)}
              >
                {arming === d.id ? t("确认断开") : t("断开")}
              </button>
            </div>
          ))}
          {st.devices.length === 0 && <p>{t("还没有手机连上来")}</p>}
        </section>
      )}

      <p className="pc-note">{t("局域网明文连接，只在可信的网络中开启")}</p>
    </div>
  );
}
