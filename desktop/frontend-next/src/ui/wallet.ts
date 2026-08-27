import { t } from "../i18n";
import type { WalletReading } from "../port/port";

/** The answers to "how much is left" that must not collapse into one number: a
 *  provider with no wallet shows nothing, a wallet that could not be read shows
 *  why, and a value past its freshness says how old it is. A zero is none of
 *  those — it is a wallet that answered and said zero. */
export type Wallet =
  | { kind: "absent" }
  | { kind: "read"; reading: WalletReading }
  | { kind: "unread"; why: string };

export const ABSENT: Wallet = { kind: "absent" };

/** How long ago, in the coarsest unit still true. A wallet moves on the order
 *  of minutes, so seconds would be precision nobody can act on. */
export function since(iso: string, now: number): string {
  const ms = now - Date.parse(iso);
  if (!Number.isFinite(ms)) return "";
  const minutes = Math.floor(Math.max(0, ms) / 60000);
  if (minutes < 1) return t("刚刚");
  if (minutes < 60) return t("{n} 分钟前", { n: minutes });
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return t("{n} 小时前", { n: hours });
  return t("{n} 天前", { n: Math.floor(hours / 24) });
}

/** The account a wallet belongs to. modelRef is "<provider>/<model>", and the
 *  provider half is the name the user gave that connection. */
export const accountOf = (modelRef?: string): string => (modelRef ?? "").split("/")[0] ?? "";
