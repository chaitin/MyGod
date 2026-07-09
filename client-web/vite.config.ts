import path from "path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

const clientApiProxyTarget =
  process.env.TARGET?.trim() || "http://localhost:20080"

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 20070,
    proxy: {
      "/api/client/ws": {
        changeOrigin: true,
        rewriteWsOrigin: true,
        secure: false,
        target: clientApiProxyTarget,
        ws: true,
      },
      "/api/client": {
        changeOrigin: false,
        secure: false,
        target: clientApiProxyTarget,
        ws: true,
      },
    },
  },
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
})
