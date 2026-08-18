import { HttpError } from "../port/port";
import { t } from "./index";

// What the kernel says when it refuses, in the language the reader uses.
//
// The kernel does not speak to people: it answers with a code and the pieces a
// sentence needs. That split is deliberate — the same refusal has to reach a
// Chinese window, an English window, a log and a curl, and only the frontend
// knows which of those it is. Wording here, decisions there.
//
// The map goes code → Chinese source text, which then runs through the ordinary
// t(): one translation mechanism for the whole app rather than a second
// catalogue keyed by codes.
const SAID: Record<string, string> = {
  // ── 忙：不是出错，是「现在不行」 ─────────────────────────────────
  "busy.switch_model": "有活儿在跑，先停下再换模型",
  "busy.change_effort": "有活儿在跑，先停下再改推理强度",
  "busy.change_workspace": "有活儿在跑，先停下再换工作区",
  "busy.reload_extensions": "有活儿在跑，先停下再重载扩展",

  // ── 冲突：有东西挡着 ─────────────────────────────────────────────
  "workspace.has_open_panes": "这个文件夹还有 {n} 个打开的面板，先关掉再移除",
  "provider.model_in_use": "这个来源正在用，先换一个模型再删",
  "hub.too_many_panes": "最多同时开 {max} 个面板，先关掉一个",

  // ── 来源：填错了什么 ─────────────────────────────────────────────
  "provider.name_required": "给这个来源起个名字",
  "provider.name_invalid": "名字只能用字母、数字、点、横线和下划线",
  "provider.endpoint_required": "填一个接口地址",
  "provider.kind_unsupported": "不认识「{kind}」这种接入方式",
  "provider.no_models_picked": "至少挑一个模型",
  "provider.default_not_selected": "默认模型「{model}」不在挑中的那几个里",

  // ── 来源：这个协议做不到 ─────────────────────────────────────────
  "provider.no_thinking_param": "这个协议不发思考参数，开了也没用",
  "provider.no_websearch_wire": "这个协议没有让端点自己搜索的写法",

  // ── 来源：连接与授权 ─────────────────────────────────────────────
  "provider.editing_disabled": "这台服务器不让改模型来源",
  "provider.key_required": "要填 API key",
  "provider.key_too_large": "这个 key 太长了，八成粘错了东西",
  "provider.setup_done": "已经连上了，不用再配一次",
  "provider.setup_failed": "远端配置没做成，稍后再试",

  // ── 会话 ─────────────────────────────────────────────────────────
  "session.disabled": "这台服务器关掉了会话切换",
  "session.pending_cleanup": "这个会话正在清理，等一下再打开",
  "session.in_use": "这个会话被别处占着，先把那边关掉",
  "session.bind_failed": "接管这个会话失败了，重开一次窗口",
  "session.has_open_pane": "这个会话还开着面板，先关掉那个",
  "session.outside_workspace": "这个路径不在任何已知的工作区里",
  "hub.no_runtime_open": "还没有打开的会话",

  // ── 壁纸 ─────────────────────────────────────────────────────────
  "wallpaper.unsupported_type": "这种图片格式用不了，换 PNG、JPEG、WebP、AVIF 或 GIF",
  "wallpaper.empty": "图片是空的",
  "wallpaper.too_large": "图片太大了，先压到 {limit} MB 以内",
};

/** Reason is what a refused request answers with. `error` is English fallback
 *  for logs and for codes this build has no wording for — never preferred over
 *  a code we do recognise. */
export interface Reason {
  code?: string;
  error?: string;
  params?: Record<string, string | number>;
}

/** say turns a kernel refusal into a sentence. An unknown code degrades to the
 *  kernel's English rather than to a blank — a message nobody translated is
 *  still better than no message. */
export function say(reason: Reason | null | undefined, fallback = ""): string {
  if (!reason) return fallback;
  const zh = reason.code ? SAID[reason.code] : undefined;
  if (zh) return t(zh, reason.params ?? {});
  return reason.error || fallback;
}

/** reason is what a catch block hands to the UI: a coded refusal becomes this
 *  window's language, anything else prints as itself. One call so no display
 *  site has to know which kind it caught. */
export function reason(e: unknown): string {
  if (e instanceof HttpError && e.reason) return say(e.reason, e.message);
  return e instanceof Error ? e.message : String(e);
}

/** codes is what the parity check reads. */
export const codes = SAID;
