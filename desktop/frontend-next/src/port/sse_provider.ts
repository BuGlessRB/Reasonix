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

  // A refused key, a path the endpoint does not serve and an embedding-only
  // gateway are three different fixes. post0 keeps the kernel's coded refusal
  // on HttpError; a bare Error would flatten it back to one string.
  probeProvider(baseUrl: string, apiKey: string): Promise<ProviderProbe> {
    return this.post0<ProviderProbe>("/providers/probe", { baseUrl, apiKey });
  }
  checkProvider(name: string): Promise<ProviderCheck> {
    return this.post0<ProviderCheck>("/providers/check", { name });
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
