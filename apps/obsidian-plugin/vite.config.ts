import { defineConfig } from "vite"
import vue from "@vitejs/plugin-vue"
import path from "path"

export default defineConfig({
  root: "dev",
  plugins: [vue()],
  resolve: {
    alias: {
      // `obsidian` ships types only (no `main`, so no entry point): the browser
      // harness must not pull the real package. Same stub vitest uses. At runtime
      // the mock leaves the app singleton unset, so Obsidian-only paths fail
      // gracefully exactly as they do today.
      obsidian: path.resolve(
        __dirname,
        "src/shared/obsidian/__mocks__/obsidian.ts",
      ),
      "@": path.resolve(__dirname, "src"),
    },
  },
  server: {
    port: 3000,
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
})
