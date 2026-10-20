// Bundles the extension into dist/: background.js, popup.js and the static
// files, with the version from package.json in manifest.json.
import { cpSync, readFileSync, writeFileSync } from "node:fs";
import * as esbuild from "esbuild";

const pkg = JSON.parse(readFileSync("package.json", "utf8"));
await esbuild.build({
  entryPoints: ["src/background.ts", "src/popup.ts"],
  bundle: true,
  outdir: "dist",
  format: "iife",
  target: "chrome116",
  minify: true,
  sourcemap: false,
  logLevel: "info",
});
cpSync("static", "dist", { recursive: true });
const manifest = JSON.parse(readFileSync("static/manifest.json", "utf8"));
manifest.version = pkg.version;
writeFileSync("dist/manifest.json", JSON.stringify(manifest, null, 2) + "\n");
