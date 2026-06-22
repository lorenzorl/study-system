import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { parse, compileScript, compileStyle } from "@vue/compiler-sfc";
import esbuild from "esbuild";

export const vuePlugin = () => ({
  name: "vue",
  setup(build) {
    build.onLoad({ filter: /\.vue$/ }, async (args) => {
      const source = readFileSync(args.path, "utf-8");
      const { descriptor } = parse(source, { filename: args.path });

      // Hash the file path to produce a valid CSS scope ID (8 hex chars).
      // Using the raw path produces selectors like [data-v-/Users/...] which
      // are invalid CSS because slashes and special chars break the selector.
      const id = createHash("md5").update(args.path).digest("hex").slice(0, 8);
      const scriptBlock = descriptor.scriptSetup || descriptor.script;

      // Compile script with inline template render function
      const script = compileScript(descriptor, {
        id,
        inlineTemplate: true,
        templateOptions: {
          id,
          filename: args.path,
          compilerOptions: { mode: "module" },
        },
      });

      let code = script.content;

      // Strip TypeScript type annotations via esbuild transform
      if (scriptBlock && scriptBlock.lang === "ts") {
        const transformed = await esbuild.transform(code, {
          loader: "ts",
          target: "es2018",
          sourcefile: args.path,
        });
        code = transformed.code;
      }

      // Compile scoped / unscoped styles and inject at module load
      let cssInjection = "";
      for (const styleBlock of descriptor.styles) {
        const compiled = compileStyle({
          source: styleBlock.content,
          id,
          filename: args.path,
          scoped: styleBlock.scoped,
        });
        if (compiled.code) {
          cssInjection +=
            `(function(){var d=document.createElement('style');` +
            `d.setAttribute('type','text/css');` +
            `d.textContent=${JSON.stringify(compiled.code)};` +
            `document.head.appendChild(d);})();\n`;
        }
      }

      return { contents: cssInjection + code, loader: "js" };
    });
  },
});
