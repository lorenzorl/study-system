/**
 * The stylesheets the deck imports as *text*, not as side-effect stylesheet
 * imports.
 *
 * Both bundlers are taught to hand these two specifiers over as a string:
 * `esbuild.config.mjs` maps `.css` to the `text` loader, and `vite.config.ts`
 * resolves exactly these ids and emits `export default <css>`. Without the Vite
 * half, Vite treats them as stylesheet imports and the default export is empty.
 * The specifiers must stay identical in both pipelines — no `?raw`, no `?inline`.
 */
declare module "reveal.js/reveal.css" {
  const css: string
  export default css
}

declare module "reveal.js/theme/black.css" {
  const css: string
  export default css
}
