import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./styles/tokens.css";
import "./styles/app.css";
import { App } from "./ui/App";
import { SsePort } from "./port/sse";
import type { AgentPort } from "./port/port";

// The dev proxy only exists when REASONIX_SERVE was set at vite start; probing
// /status decides which port to boot on, so neither mode needs a build flag.
// Without the proxy vite answers /status with the SPA shell at 200, so the
// content type — not res.ok — is what says a kernel is really there.
async function pick(): Promise<AgentPort> {
  try {
    const res = await fetch("/status", { credentials: "same-origin" });
    if (res.ok && (res.headers.get("content-type") ?? "").includes("json")) return new SsePort();
  } catch {
    // no serve reachable
  }
  // A shipped build is served by the kernel it talks to. Falling back to the
  // fixture there would put a scripted session on screen as if it had happened,
  // so only a dev build is allowed to; the import stays dynamic to keep the
  // fixture out of the production bundle.
  if (!import.meta.env.DEV) throw new Error("连不上内核：/status 没有回应。");
  const { MockPort } = await import("./port/mock");
  return new MockPort();
}

// The shell hides the native title bar and lets its lights float over the page,
// so the chrome has to reserve their corner and be draggable itself. Neither is
// true of the browser build, and only macOS puts them on the left — so the page
// is told which shell it is in rather than assuming one.
interface WailsEnv {
  runtime?: { Environment?: () => Promise<{ platform?: string }> };
}
(window as unknown as WailsEnv).runtime?.Environment?.()
  .then((env) => {
    document.documentElement.dataset.shell = "wails";
    if (env.platform) document.documentElement.dataset.platform = env.platform;
  })
  .catch(() => {});

// macOS hides its traffic lights outright on an inactive window rather than
// greying them, so the corner they were reserved is simply empty. The wordmark
// takes the slot back while they are gone; the slot itself never resizes.
const focus = () => {
  document.documentElement.dataset.focused = document.hasFocus() ? "yes" : "no";
};
addEventListener("focus", focus);
addEventListener("blur", focus);
focus();

const root = createRoot(document.getElementById("root")!);
pick().then(
  (port) =>
    root.render(
      <StrictMode>
        <App port={port} />
      </StrictMode>,
    ),
  (e: unknown) =>
    root.render(
      <div className="app" data-run="idle">
        <div className="errbar" role="alert">
          <span>{e instanceof Error ? e.message : String(e)}</span>
        </div>
      </div>
    ),
);
