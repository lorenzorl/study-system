import { defineConfig } from "vitest/config"
import vue from "@vitejs/plugin-vue"
import path from "node:path"

export default defineConfig({
  // The Vue plugin lets component tests render through `vue/server-renderer`.
  // It changes nothing for the pure-model suites.
  plugins: [vue()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  test: {
    globals: true,
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
})
