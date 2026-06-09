import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// Dev proxy /api → :8080 (PRD §5): mutations, the authoritative read, and
// the SSE stream all hit the Go server; Vite streams SSE without buffering
// by default.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        // When the Go server dies mid-stream, http-proxy leaves the
        // browser's SSE connection hanging open (the drop surfaces as
        // proxyRes "aborted"/"close", not the top-level "error" event) —
        // EventSource never errors and never reconnects. Destroy the client
        // response so the browser sees the drop, like it would against the
        // origin in production (F6's reconnect contract depends on this).
        configure(proxy) {
          proxy.on("proxyRes", (proxyRes, _req, res) => {
            proxyRes.on("close", () => {
              if (!res.writableEnded) {
                res.destroy()
              }
            })
          })
        },
      },
    },
  },
})
