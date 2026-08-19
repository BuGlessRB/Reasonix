import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { boot as bootLang } from "./i18n";
import { track as trackWidth } from "./ui/viewport";
import "./styles/tokens.css";
import "./styles/app.css";
import { App } from "./ui/App";
import { SseHub } from "./port/hub";
import type { HubPort } from "./port/hub";
import { install as installFileDrop } from "./ui/filedrop";

// The dev proxy only exists when REASONIX_SERVE was set at vite start; probing
// /status decides which port to boot on, so neither mode needs a build flag.
// Without the proxy vite answers /status with the SPA shell at 200, so the
// content type — not res.ok — is what says a kernel is really there.
async function pick(): Promise<HubPort> {
  try {
    const res = await fetch("/status", { credentials: "same-origin" });
    if (res.ok && (res.headers.get("content-type") ?? "").includes("json")) return new SseHub();
  } catch {
    // no serve reachable
  }
  // A shipped build is served by the kernel it talks to. Falling back to the
  // fixture there would put a scripted session on screen as if it had happened,
  // so only a dev build is allowed to; the import stays dynamic to keep the
  // fixture out of the production bundle.
  if (!import.meta.env.DEV) throw new Error("连不上内核：/status 没有回应。");
  const { MockHub } = await import("./port/mock_hub");
  return new MockHub();
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

// Before the first paint: a language that arrives after one would show the
// interface in one language and then swap it.
bootLang();
trackWidth();

const root = createRoot(document.getElementById("root")!);
pick().then(
  (hub) => {
    // Before the first render, and not from whichever view happens to want a
    // drop: a window with no drop target mounted still has to refuse a file, or
    // the webview navigates to it and the app is replaced by what was dropped.
    installFileDrop(hub);
    root.render(
      <StrictMode>
        <App hub={hub} />
      </StrictMode>,
    );
  },
  (e: unknown) =>
    root.render(
      <div className="app" data-run="idle">
        <div className="errbar" role="alert">
          <span>{e instanceof Error ? e.message : String(e)}</span>
        </div>
      </div>
    ),
);
