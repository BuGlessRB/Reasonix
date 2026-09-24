import { t } from "../i18n";
import type { BrowserLoadFailure } from "../port/host";

// What a Chromium net error means for someone who can act on it, read from the
// error's name. Electron draws no error page, so without this a failed load is
// a blank view with no reason at all.
function explain(reason: string): string {
  if (reason.startsWith("ERR_CERT_") || reason === "ERR_SSL_PINNED_KEY_NOT_IN_CERT_CHAIN") {
    return t("这个网站的证书不受信任，常见于内网的自签名证书或公司私有证书。");
  }
  if (reason.startsWith("ERR_SSL_") || reason === "ERR_BAD_SSL_CLIENT_AUTH_CERT") {
    return t("无法建立安全连接。这个网站可能只支持 http，或者要求客户端证书。");
  }
  if (reason.startsWith("ERR_PROXY_") || reason.startsWith("ERR_TUNNEL_")) {
    return t("连不上系统代理。内置浏览器使用系统的代理设置，而不是 Reasonix 设置里的网络代理。");
  }
  switch (reason) {
    case "ERR_NAME_NOT_RESOLVED":
    case "ERR_NAME_RESOLUTION_FAILED":
      return t("找不到这个地址。内网域名需要连上公司网络或 VPN 才能解析。");
    case "ERR_CONNECTION_REFUSED":
      return t("对方拒绝了连接。请确认端口正确、服务已经启动。");
    case "ERR_CONNECTION_TIMED_OUT":
    case "ERR_TIMED_OUT":
    case "ERR_ADDRESS_UNREACHABLE":
      return t("连接超时或地址不可达。请确认已连上公司网络或 VPN。");
    case "ERR_INVALID_AUTH_CREDENTIALS":
      return t("登录没有通过，请重试并检查用户名和密码。");
    default:
      return t("网页没有加载成功。");
  }
}

interface Props {
  failure: BrowserLoadFailure;
  onRetry: () => void;
  onExternal: () => void;
}

/** BrowserFailure takes the place of a page that did not load, and says why. */
export function BrowserFailure({ failure, onRetry, onExternal }: Props) {
  return (
    <div className="bfail" role="alert">
      <b>{t("这个网页打不开")}</b>
      <p>{explain(failure.reason)}</p>
      <code>
        {failure.reason || failure.code} · {failure.url}
      </code>
      <div className="bfail-acts">
        <button className="btn sm" data-action="browser.control" data-value="retry" onClick={onRetry}>
          {t("重试")}
        </button>
        <button className="btn sm" data-action="browser.external" onClick={onExternal}>
          {t("在外部浏览器打开")}
        </button>
      </div>
    </div>
  );
}
