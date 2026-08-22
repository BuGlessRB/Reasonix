import { SseNetwork } from "./sse_network";
import type { ThemePack } from "./port";

// Theme packs, and where an extension's view is allowed to sit. Both are the
// user overruling a default the pack or the extension asked for.
export class SseTheme extends SseNetwork {
  themes() {
    return this.get<ThemePack[]>("/themes");
  }
  activateTheme(id: string) {
    return this.post("/themes", { id });
  }
  // The extension's own message is the result, so this reads the body rather
  // than a status code. A refused action answers 422 with its reason.
  async surfaceSlots() {
    const r = await this.get<{ slots?: Record<string, string> }>("/surfaces");
    return r.slots ?? {};
  }
  assignSurface(surface: string, slot: string) {
    return this.post("/surfaces", { surface, slot });
  }
}
