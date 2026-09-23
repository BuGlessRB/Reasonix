import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { boot as bootLang } from "./i18n";
import { reason } from "./i18n/kernel";
import { track as trackWidth } from "./ui/viewport";
import "./styles/tokens.css";
import "./styles/app.css";
import "./styles/studio.css";
import { App } from "./ui/App";
import { SseHub } from "./port/hub";
import type { HubPort } from "./port/hub";
import { install as installFileDrop } from "./ui/filedrop";
import { host } from "./port/host";
import { play } from "./boot/intro";
import { settled } from "./boot/gate";

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

// A shell that hides the native title bar lets its lights float over the page,
// so the chrome has to reserve their corner and be draggable itself. Which of
// those is true is the shell's answer, not something the page infers from a
// platform or from which globals it can see.
void host()
  .describe()
  .then(({ shell, platform, titleBar }) => {
    const root = document.documentElement.dataset;
    root.shell = shell;
    if (platform) root.platform = platform;
    if (titleBar) root.titlebar = "app";
  });

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

// The boot screen is markup in index.html, so it is on screen before this
// bundle is parsed; from here it plays the intro while the kernel is asked.
const intro = play(document.getElementById("boot"));
intro.progress(1);
// A shell that never settles — no pane to ask on — must not keep the window
// behind the wordmark forever.
const SETTLE_CAP_MS = 6000;

// The window is handed over only when both are true: the intro has put the
// wordmark down, and the app has something of its own to show. Whichever is
// later decides; until then the wordmark holds under its glint.
function arrive(shown: Promise<unknown>) {
  const root = document.documentElement;
  const cap = new Promise((done) => setTimeout(done, SETTLE_CAP_MS));
  void Promise.all([intro.held, Promise.race([shown, cap])])
    .then(() => new Promise(requestAnimationFrame))
    .then(() => {
      // Arms the app's own entrance under the opening hole, then lets go of
      // it: a .app that remounts later must not replay the window opening.
      root.dataset.boot = "in";
      setTimeout(() => delete root.dataset.boot, 2200);
      return intro.leave();
    })
    .then(() => document.getElementById("boot")?.remove());
}

const root = createRoot(document.getElementById("root")!);
pick().then(
  (hub) => {
    // Before the first render, and not from whichever view happens to want a
    // drop: a window with no drop target mounted still has to refuse a file, or
    // the webview navigates to it and the app is replaced by what was dropped.
    installFileDrop();
    root.render(
      <StrictMode>
        <App hub={hub} />
      </StrictMode>,
    );
    intro.progress(2);
    void settled.then(() => intro.progress(3));
    arrive(settled);
  },
  (e: unknown) => {
    root.render(
      <div className="app" data-run="idle">
        <div className="errbar" role="alert">
          <span>{reason(e)}</span>
        </div>
      </div>,
    );
    // The failure is the one thing that must not stay behind the boot screen.
    arrive(Promise.resolve());
  },
);
