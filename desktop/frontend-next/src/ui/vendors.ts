// vendors.ts — one account, however many doors it answers on.
//
// Two config entries at one host are one service reached under two protocols,
// not two services. Both the connection list and the model list group on that,
// so they have to agree on what counts as the same host and what to call it.

export const KIND_LABEL: Record<string, string> = {
  openai: "OpenAI 兼容",
  anthropic: "Anthropic 兼容",
  responses: "Responses",
  extension: "扩展",
};

export function hostOf(baseUrl: string): string {
  try {
    return new URL(baseUrl).hostname.toLowerCase();
  } catch {
    return baseUrl.trim().toLowerCase();
  }
}

// The name a person would use for the account. "api.deepseek.com" is deepseek —
// the config's own entry names are not usable here, because the whole problem is
// that one account owns several of them.
export function vendorLabel(host: string): string {
  const bare = host.replace(/^(www|api|open|gateway)\./, "");
  const first = bare.split(".")[0];
  return first || host;
}
