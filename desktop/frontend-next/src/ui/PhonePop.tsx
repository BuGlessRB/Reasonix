import { useCallback, useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import type { HubPort } from "../port/hub";
import { useDismiss } from "./dismiss";
import { ShareBody, useShare } from "./PhoneAccess";

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
      {open && (
        <div className="phonecard share" role="dialog" aria-label={t("手机扫码访问")}>
          <div className="phonecard-hd">
            <b>{t("手机扫码访问")}</b>
          </div>
          <ShareBody share={share} />
        </div>
      )}
    </div>
  );
}
