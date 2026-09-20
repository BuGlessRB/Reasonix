import assert from "node:assert/strict";
import { managementDom } from "../test-support/managementDom";
import { installDesktopHostStub } from "./desktopHostStub";

const dom = managementDom();
const { default: React, act } = await import("react");
const { createRoot } = await import("react-dom/client");
const { LocaleProvider } = await import("../lib/i18n");
const { HistoricalSessionBanners } = await import("../components/SessionTakeoverDialog");
const { setHistoricalPreparation } = await import("../app-runtime/desktopNavigationOwner");
let cancelled = 0;
const opened: string[] = [];
const source = { hostId: "local", sourceKey: "legacy", path: "/fixture/legacy.jsonl" };
const host = installDesktopHostStub({
  CheckHistoricalSourceUpdate: async () => ({ sourceKey: "legacy", status: "available", version: "v2", source, retryable: false }),
  PrepareHistoricalSourceVersion: async () => ({ operationId: "version-v2", sourceKey: "legacy", status: "ready", revision: 2,
    target: { hostId: "local", sessionId: "branch-v2" }, retryable: false }),
  GetSessionPreparation: async () => { throw new Error("terminal preparation must not poll"); },
  CancelSessionPreparation: async (operationId: string) => { cancelled++; return { operationId, sourceKey: "legacy", status: "cancelled", revision: 3, retryable: true }; },
});
const root = createRoot(document.getElementById("root")!);
const baseProps = {
  tab: { id: "base", scope: "global", workspaceRoot: "", workspaceName: "Global", topicId: "base", topicTitle: "Base",
    label: "Base", ready: true, running: false, sessionId: "base" },
  navigate: async (intent: { kind: string; ref?: { sessionId: string } }) => { if (intent.ref) opened.push(intent.ref.sessionId); },
};
setHistoricalPreparation({
  operationId: "prepare-legacy", status: "queued", retryable: false,
  session: { scope: "global", title: "Legacy title", topicId: "legacy", source },
});
await act(async () => root.render(<LocaleProvider><HistoricalSessionBanners {...baseProps} /></LocaleProvider>));
assert.ok(document.body.textContent?.includes("Importing: Legacy title"));
await act(async () => [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent === "Cancel")!.click());
assert.equal(cancelled, 1);

await act(async () => setHistoricalPreparation(null));
await act(async () => root.render(<LocaleProvider><HistoricalSessionBanners {...baseProps} /></LocaleProvider>));
await act(async () => {});
assert.ok(document.body.textContent?.includes("Historical sessions · Not imported"));
await act(async () => [...document.querySelectorAll<HTMLButtonElement>("button")].find(button => button.textContent === "Import and open · Branch")!.click());
assert.deepEqual(opened, ["branch-v2"]);
await act(async () => root.unmount());
host.uninstall();
dom.window.close();
console.log("PASS preparation banner, cancellation and source update branch import");
