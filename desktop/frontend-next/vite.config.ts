import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import { ISOLATED } from "./src/testing/isolated.ts";

interface ProxyEvents {
  on(event: "proxyReq", cb: (req: { setHeader(k: string, v: string): void }) => void): void;
}

// Prefixes: vite matches a proxy key against the head of the path, so "/mcp"
// also carries /mcp/reconnect. A route missing here answers with the SPA shell
// instead of JSON, which is indistinguishable from a broken endpoint.
const ROUTES = [
  // The hub's own surface, plus /rt — every pane's requests carry that prefix,
  // so without it a second session's traffic answers with the SPA shell.
  "/runtimes", "/tree", "/rt", "/remotes",
  "/appearance",
  "/events", "/history", "/status", "/balance", "/submit", "/cancel", "/approve", "/answer",
  "/adjudications", "/execution-graph",
  "/plan", "/plan-decision", "/goal", "/resume", "/models", "/tool-approval-mode", "/preset",
  "/model", "/effort", "/new", "/sessions", "/delete-session", "/provider-setup",
  "/inbox", "/trajectory", "/mcp", "/skills", "/complete", "/workspace", "/capability-scope",
  "/providers", "/decision-models", "/roles", "/account", "/hooks", "/memory", "/network", "/shell", "/todos",
  "/changes", "/attachments", "/drop", "/checkpoints", "/branches", "/compact", "/compaction", "/rewind",
  "/extensions", "/themes", "/plugins", "/surfaces",
  "/fork", "/summarize", "/forget", "/bypass", "/auto-approve-tools",
  "/permissions", "/sandbox", "/context", "/storage", "/tray", "/browser", "/browser-host", "/asks", "/update",
  "/host", "/notifications",
  "/slash", "/workspaces", "/welcome", "/usage", "/config", "/studio",
];

// REASONIX_SERVE points at a running `reasonix serve`; without it the app boots
// on MockPort so the UI can be developed with no Go process at all.
export default defineConfig(({ mode }) => {
  const serve = loadEnv(mode, ".", "REASONIX").REASONIX_SERVE;
  return {
    base: "./",
    plugins: [react()],
    // Stylesheets are stubbed for tests by default, `?raw` included, so a
    // guard that reads app.css to check a rule gets an empty string and passes
    // on nothing. Processing it is what makes that guard able to fail.
    //
    // One worker per thread, reused across files: a fresh module registry per
    // file meant re-importing React, the port and app.css once each, which was
    // most of the run. The cost is that module-level state now outlives a file,
    // so a test that needs a clean one has to make it, not assume it.
    //
    // A file calling vi.mock cannot share that registry — a module another file
    // already imported unmocked stays unmocked — so those run isolated. The list
    // is checked against the sources by src/testing/isolated.test.ts, which is
    // what keeps it from quietly stopping to match.
    test: {
      css: true,
      projects: [
        { extends: true, test: { name: "shared", pool: "threads", isolate: false, exclude: [...ISOLATED, "**/node_modules/**", "**/dist/**"] } },
        { extends: true, test: { name: "mocked", pool: "threads", isolate: true, include: [...ISOLATED] } },
      ],
    },
    server: {
      port: 5273,
      // The dev server is cross-origin to serve, which its CSRF guard rejects.
      // Rewriting Origin makes the hop look same-origin, matching production
      // where the UI really is served from the kernel.
      proxy: serve
        ? Object.fromEntries(
            ROUTES.map((r) => [
              r,
              {
                target: serve,
                changeOrigin: true,
                configure: (proxy: unknown) => {
                  (proxy as ProxyEvents).on("proxyReq", (req) => req.setHeader("origin", serve));
                },
              },
            ]),
          )
        : undefined,
    },
    build: {
      outDir: "dist",
      emptyOutDir: true,
      rolldownOptions: {
        output: {
          codeSplitting: {
            groups: [
              {
                name: "react-runtime",
                test: /node_modules[\\/](?:react|react-dom|scheduler)[\\/]/,
                priority: 10,
              },
            ],
          },
        },
      },
    },
  };
});
