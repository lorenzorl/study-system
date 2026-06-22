import esbuild from "esbuild";
import { vuePlugin } from "../esbuild-vue-plugin.mjs";

const ctx = await esbuild.context({
  entryPoints: ["dev/main.ts"],
  bundle: true,
  format: "esm",
  target: "es2020",
  platform: "browser",
  outdir: "dev/dist",
  sourcemap: "inline",
  plugins: [vuePlugin()],
  alias: {
    vue: "vue/dist/vue.runtime.esm-bundler.js",
  },
  banner: {
    js: "/* Dev preview — not for Obsidian */",
  },
});

await ctx.watch();

const { host, port } = await ctx.serve({
  servedir: "dev",
  port: 3000,
});

console.log(`Dev preview running at http://${host}:${port}`);
