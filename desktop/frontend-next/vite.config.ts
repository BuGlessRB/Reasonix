import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

interface ProxyEvents {
  on(event: "proxyReq", cb: (req: { setHeader(k: string, v: string): void }) => void): void;
}

const ROUTES = [
  "/events", "/history", "/status", "/submit", "/cancel", "/approve", "/answer",
  "/plan", "/goal", "/resume", "/models", "/tool-approval-mode", "/preset",
  "/model", "/effort", "/new", "/sessions", "/delete-session", "/provider-setup",
  "/inbox", "/trajectory",
];

// REASONIX_SERVE points at a running `reasonix serve`; without it the app boots
// on MockPort so the UI can be developed with no Go process at all.
export default defineConfig(({ mode }) => {
  const serve = loadEnv(mode, ".", "REASONIX").REASONIX_SERVE;
  return {
    base: "./",
    plugins: [react()],
    server: {
      port: 5273,
      // The dev server is cross-origin to serve, which its CSRF guard rejects.
      // Rewriting Origin makes the hop look same-origin, matching production
      // and the Wails shell where the UI really is served from the kernel.
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
    build: { outDir: "dist", emptyOutDir: true },
  };
});
