import { defineConfig, type Plugin } from "vite"
import vue from "@vitejs/plugin-vue"
import tailwindcss from "@tailwindcss/vite"
import { readFileSync } from "node:fs"
import { createRequire } from "node:module"
import path from "node:path"

// Resolves the deck's stylesheet specifiers from this app's own node_modules
// instead of hard-coding a pnpm store path.
const requireFromApp = createRequire(path.join(__dirname, "vite.config.ts"))

/**
 * Reveal's stylesheets are imported as *text* by `DeckViewer.vue`, which puts
 * them inside the deck's shadow root. Vite would otherwise treat those imports
 * as side-effect stylesheet links, inject them into the document and give the
 * module an empty default export — the deck would mount with no styles, and the
 * app document would be styled by a stylesheet the deck was supposed to own.
 */
function revealStylesheetAsText(): Plugin {
  // The virtual ids deliberately carry no `.css` suffix: Vite keys its CSS
  // pipeline on the id, and an id ending in `.css` would be parsed as a
  // stylesheet — `export default "..."` is not valid CSS.
  const prefix = "\0reveal-css-text:"
  const stylesheets = ["reveal.js/reveal.css", "reveal.js/theme/black.css"] as const

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
      const css = readFileSync(requireFromApp.resolve(specifier), "utf8")
      return `export default ${JSON.stringify(css)}`
    },
  }
}

export default defineConfig({
  // `tailwindcss()` compiles `src/app/styles.css` (the token bridge) and the
  // utilities the `shared/ui` primitives use. It is a Vite plugin, not a
  // PostCSS step: there is no `postcss.config.js` and no `tailwind.config.js`,
  // because in v4 the config *is* the CSS file.
  plugins: [revealStylesheetAsText(), vue(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  server: {
    port: 5173,
    // Same-origin in development: the app requests `/api/...` and Vite forwards
    // it, so the browser never makes a cross-origin request and the server's
    // CORS allowlist stays as narrow as it is.
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
})
