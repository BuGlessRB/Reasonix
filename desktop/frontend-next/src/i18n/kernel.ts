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
  "request.bad_body": "这次请求的内容读不出来，刷新页面再试一次",
  "request.missing_field": "还缺一个「{field}」",
  "request.not_found": "找不到叫「{name}」的{kind}",
  "project.unknown": "这个项目没有在当前窗口打开",
  "busy.session_in_use": "这个会话正被别处占用：{detail}",
  "busy.session_running": "这个会话正在跑，{detail}",
  "busy.session_active": "这就是当前打开的会话，先切走再删",
  "request.bad_value": "「{field}」只能是这几个之一：{allowed}",
  "session.bad_name": "会话名不能是路径",
  "session.bad_path": "这个会话路径解析不出来",
  "session.outside_dir": "这个路径在会话目录之外",
  "request.method_not_allowed": "这个地址不接受这种请求方式",
  "request.bad_content_type": "请求体必须是 application/json",
  "permissions.editing_disabled": "这台服务器没有开放权限编辑",
  "sandbox.editing_disabled": "这台服务器没有开放沙箱编辑",
  "roles.editing_disabled": "这台服务器没有开放角色编辑",
  "storage.moving_disabled": "这台服务器没有开放搬迁存储",
  "storage.move_running": "已经有一个搬迁在跑了，等它结束",
  "plugin.bad_name": "这不是一个插件名",
  "plugin.not_installed": "这个插件没有安装",
  "wallpaper.not_base64": "图片数据不是 base64",
  "shell.unavailable_over_http": "HTTP 上不提供 shell 命令",
  "roles.unknown": "没有「{role}」这个角色",
  "roles.model_unknown": "没有配置好的模型能匹配「{model}」",
  "shell.editing_disabled": "这台服务器没有开放 shell 设置",
  "account.signin_disabled": "这台服务器没有开放账号登录",
  "workspace.changing_disabled": "这台服务器不能切换工作区",
  "settings.unknown_preset": "没有这个预设",
  "drop.too_many_paths": "一次拖入 {count} 个，最多 {limit} 个",
  "complete.line_too_long": "这一行太长，补全不了",
  "stream.unsupported": "这条连接不支持流式传输",
  "internal.failed": "这边出了点问题，不是你的操作有误",
  "provider.bad_context_window": "上下文长度不能是负数；填 0 表示不自动压缩",
  "provider.bad_reasoning_protocol": "不认识「{protocol}」这种思考协议",
  "provider.extra_body_null": "额外设置里「{path}」不能是空值（null）",
  "provider.no_websearch_wire": "这个协议没有让端点自己搜索的写法",

  // ── 来源：连接与授权 ─────────────────────────────────────────────
  "provider.editing_disabled": "这台服务器不让改模型来源",
  "provider.key_required": "要填 API key",
  "provider.key_too_large": "这个 key 太长了，八成粘错了东西",
  "provider.setup_done": "已经连上了，不用再配一次",
  "provider.setup_failed": "远端配置没做成，稍后再试",

  // ── 推理强度：这个端点给不了 ─────────────────────────────────────
  "effort.not_configurable":
    "{provider} 没说自己有哪些推理强度档位。要有，得在它的配置块里写 reasoning_protocol 或 supported_efforts",
  "effort.unsupported_level": "{provider} 没有「{level}」这一档，能选的是：{levels}",
  "effort.no_provider": "认不出现在用的是哪个来源，先切一次模型",

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
