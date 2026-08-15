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

// The account a route belongs to. Host alone is not enough: a relay reached
// with two different keys is two accounts — personal beside work, or two
// tenants of the same gateway — and merging them would show one balance and one
// key state for both.
export function accountKey(host: string, keyEnv?: string): string {
  return `${host}\u0000${(keyEnv ?? "").trim()}`;
}

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

// Two accounts on one host are both called by that host's name, which tells the
// user nothing about which is which. Only then does the config entry's own name
// earn a place on screen.
export function disambiguate<T extends { host: string; label: string; hint: string }>(items: T[]): T[] {
  const seen = new Map<string, number>();
  for (const it of items) seen.set(it.host, (seen.get(it.host) ?? 0) + 1);
  return items.map((it) =>
    (seen.get(it.host) ?? 0) > 1 && it.hint ? { ...it, label: `${it.label} · ${it.hint}` } : it,
  );
}
