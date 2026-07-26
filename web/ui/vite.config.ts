import { writeFileSync } from "node:fs"
import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig, type Plugin } from "vite"

// emptyOutDir wipes web/dist including the tracked .gitkeep that keeps the
// go:embed directive compiling on a clean checkout; restore it after building.
function keepGitkeep(): Plugin {
  return {
    name: "restore-gitkeep",
    closeBundle() {
      writeFileSync(path.resolve(__dirname, "../dist/.gitkeep"), "")
    },
  }
}

// The build lands in web/dist, which the Go binary embeds via go:embed
// (web/embed.go). In development the Go server keeps owning the JSON API, so
// /api is proxied to it; everything else is Vite's hot-reloading dev server.
export default defineConfig({
  plugins: [react(), tailwindcss(), keepGitkeep()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    outDir: "../dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8888",
        changeOrigin: true,
      },
    },
  },
})
