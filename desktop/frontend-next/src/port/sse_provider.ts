import { SseBoundary } from "./sse_boundary";
import type { Protocol, ProviderCheck, ProviderDraft, ProviderEdit, ProviderEntry, ProviderProbe } from "./port";

// Where models come from: the accounts, the protocols their endpoints answer,
// and what probing one found.
export class SseProvider extends SseBoundary {
  providers() {
    return this.get<ProviderEntry[]>("/providers");
  }
  protocols() {
    return this.get<Protocol[]>("/providers/protocols");
  }

  // The failure message is the answer here — a 401, a wrong path and "no chat
  // models" send the user to three different fixes — so it is read out of the
  // body rather than thrown away as a status code.
  // The failure message is the answer here — a 401, a wrong path and "no chat
  // models" send the user to three different fixes — so it is read out of the
  // body rather than thrown away as a status code.
  async probeProvider(baseUrl: string, apiKey: string): Promise<ProviderProbe> {
    const res = await fetch(this.base + "/providers/probe", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ baseUrl, apiKey }),
    });
    const text = await res.text();
    if (!res.ok) throw new Error(text.trim() || `/providers/probe: ${res.status}`);
    return JSON.parse(text) as ProviderProbe;
  }
  // Like the add-a-source probe, the interesting answer is in the body: a
  // refused key and a moved endpoint are different fixes.
  async checkProvider(name: string): Promise<ProviderCheck> {
    const res = await fetch(this.base + "/providers/check", {
      method: "POST",
      headers: { "content-type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify({ name }),
    });
    const text = await res.text();
    if (!res.ok) throw new Error(text.trim() || `/providers/check: ${res.status}`);
    return JSON.parse(text) as ProviderCheck;
  }
  saveProvider(draft: ProviderDraft) {
    return this.post("/providers", draft);
  }
  setProviderWebSearch(name: string, on: boolean) {
    return this.post("/providers/websearch", { name, on });
  }
  setProviderThinking(name: string, on: boolean) {
    return this.post("/providers/thinking", { name, on });
  }
  editProvider(edit: ProviderEdit) {
    return this.post("/providers/edit", edit);
  }
  removeProvider(name: string) {
    return this.post("/providers/remove", { name });
  }
}
