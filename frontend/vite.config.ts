import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
  cacheDir: process.env.VITE_CACHE_DIR ?? "node_modules/.vite",
  server: {
    host: "0.0.0.0",
    port: 5173,
    strictPort: true,
  },
})

