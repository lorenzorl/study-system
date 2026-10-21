import { defineConfig, type Plugin } from "vite"
import vue from "@vitejs/plugin-vue"
import { readFileSync } from "node:fs"
import { createRequire } from "node:module"
import path from "path"

// Resolves the deck's stylesheet specifiers from the plugin's own node_modules,
// exactly as esbuild does, instead of hard-coding a pnpm store path.
const requireFromPlugin = createRequire(path.join(__dirname, "vite.config.ts"))

/**
 * Reveal's stylesheets are imported as *text* by `DeckViewer.vue`, which puts
 * them inside the deck's shadow root. Vite would otherwise treat those imports
 * as side-effect stylesheet links, inject them into the harness document and give
 * the module an empty default export — the deck would mount with no styles, and
 * the harness would style a page that the plugin never renders that way.
 *
 * `esbuild.config.mjs` implements the same contract with `loader: { ".css":
 * "text" }`. Both halves must agree on the specifiers: no `?raw`, no `?inline`.
 */
function revealStylesheetAsText(): Plugin {
  // The virtual ids deliberately carry no `.css` suffix: Vite keys its CSS
  // pipeline on the id, and an id ending in `.css` would be parsed as a
  // stylesheet — `export default "..."` is not valid CSS.
  const prefix = "\0reveal-css-text:"
  const stylesheets = [
    "reveal.js/reveal.css",
    "reveal.js/theme/black.css",
  ] as const

  return {
    name: "reveal-stylesheet-as-text",
    // Before Vite's own CSS handling, in serve and in build.
    enforce: "pre",
    resolveId(id) {
      const index = stylesheets.indexOf(id as (typeof stylesheets)[number])
      return index === -1 ? null : prefix + index
    },
    load(id) {
      if (!id.startsWith(prefix)) return null

      const specifier = stylesheets[Number(id.slice(prefix.length))]
      const css = readFileSync(requireFromPlugin.resolve(specifier), "utf8")
      return `export default ${JSON.stringify(css)}`
    },
  }
}

export default defineConfig({
  root: "dev",
  plugins: [revealStylesheetAsText(), vue()],
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
