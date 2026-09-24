import { t } from "../i18n";
import type { Via } from "../port/wire";
import type { DeviceSelf } from "../port/share";

/** Where a message came from, said only when it was not this screen: another
 *  device by its number, or the window when this page is a paired device. A
 *  screen's own messages carry nothing, as they always have. */
export function messageSource(via: Via | undefined, viewer: DeviceSelf | null): string {
  if (via) return viewer?.id === via.device ? "" : t("来自 设备 {n}", { n: via.ordinal });
  return viewer ? t("来自 电脑") : "";
}

/** Who settled a prompt this screen did not answer. */
export function decisionSource(by: Via | "window" | undefined, viewer: DeviceSelf | null): string {
  if (!by) return "";
  if (by === "window") return viewer ? t("在电脑上处理") : "";
  return viewer?.id === by.device ? "" : t("由 设备 {n} 处理", { n: by.ordinal });
}
