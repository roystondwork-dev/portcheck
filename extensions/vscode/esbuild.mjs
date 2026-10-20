// Bundles the extension into dist/extension.js (CommonJS, Node target), and
// copies the .portcheck.yaml JSON Schema next to it for YAML validation.
import { copyFileSync, mkdirSync } from "node:fs";
import * as esbuild from "esbuild";

mkdirSync("dist", { recursive: true });
copyFileSync("../../docs/schema/portcheck.schema.json", "dist/portcheck.schema.json");

const watch = process.argv.includes("--watch");
const ctx = await esbuild.context({
  entryPoints: ["src/extension.ts"],
  bundle: true,
  outfile: "dist/extension.js",
  external: ["vscode"],
  format: "cjs",
  platform: "node",
  target: "node20",
  sourcemap: true,
  minify: !watch,
  logLevel: "info",
});
if (watch) {
  await ctx.watch();
} else {
  await ctx.rebuild();
  await ctx.dispose();
}
