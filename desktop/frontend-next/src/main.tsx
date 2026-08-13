import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./styles/tokens.css";
import "./styles/app.css";
import { App } from "./ui/App";
import { MockPort } from "./port/mock";
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
  return new MockPort();
}

pick().then((port) => {
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <App port={port} />
    </StrictMode>,
  );
});
