import { SseLook } from "./sse_look";
import type { PluginExport, PluginInstallRequest, PluginPackage, PluginPlan, ScopeLayer, SkillCatalog } from "./port";
import { rootQuery } from "./sse_http";
import { download } from "./download";
import { host } from "./host";

// Skills and plugin packages: what a project brought with it, and the two acts
// that change that — installing one and switching one off.
export class SseExtensions extends SseLook {
  skills(root?: string) {
    return this.get<SkillCatalog>("/skills" + rootQuery(root));
  }
  reloadExtensions() {
    return this.post("/extensions/reload");
  }
  setSkillEnabled(name: string, enabled: boolean, scope: ScopeLayer = "project", root?: string) {
    return this.post("/skills/enabled", { name, enabled, scope, root });
  }
  clearSkillOverride(name: string, root?: string) {
    return this.post("/skills/enabled", { name, clear: true, scope: "project", root });
  }
  plugins() {
    return this.get<PluginPackage[]>("/plugins");
  }
  planPlugin(req: PluginInstallRequest) {
    return this.post0<PluginPlan>("/plugins/plan", req);
  }
  installPlugin(req: PluginInstallRequest) {
    return this.post0<PluginPlan>("/plugins/install", req);
  }
  async setPluginEnabled(name: string, enabled: boolean) {
    await this.post0<{ reloadError?: string }>("/plugins/enabled", { name, enabled });
  }
  removePlugin(name: string): Promise<PluginPlan> {
    return this.del0<PluginPlan>("/plugins/" + encodeURIComponent(name));
  }

  // The header is read before the body because the body is bytes and has
  // nowhere to say what was stripped out of it.
  async exportPlugin(name: string): Promise<PluginExport> {
    const res = await fetch(this.base + "/plugins/" + encodeURIComponent(name) + "/export", {
      credentials: "same-origin",
    });
    if (!res.ok) throw new Error(`/plugins/${name}/export: ${res.status}`);
    const required = (res.headers.get("X-Reasonix-Required-Env") ?? "").split(",").filter(Boolean);
    const url = URL.createObjectURL(await res.blob());
    const a = document.createElement("a");
    a.href = url;
    a.download = `${name}.zip`;
    a.click();
    URL.revokeObjectURL(url);
    return { required };
  }
  async saveText(name: string, content: string): Promise<string | null> {
    const saved = await host().saveText(name, content);
    // null is a shell with no save surface at all; "" is the dialog dismissed,
    // and only the first of those is a reason to fall back to the browser's.
    if (saved !== null) return saved || null;
    download(name, content);
    return null;
  }
}
