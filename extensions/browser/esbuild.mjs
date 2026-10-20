// Bundles the extension: background.js, popup.js and the static files, with
// the version from package.json in manifest.json.
//   node esbuild.mjs          → dist/       (release zip; the manifest key pins its ID)
//   node esbuild.mjs --store  → dist-store/ (store upload; stores assign the ID)
import { cpSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import * as esbuild from "esbuild";

const store = process.argv.includes("--store");
const outdir = store ? "dist-store" : "dist";
const pkg = JSON.parse(readFileSync("package.json", "utf8"));

rmSync(outdir, { recursive: true, force: true });
await esbuild.build({
  entryPoints: ["src/background.ts", "src/popup.ts"],
  bundle: true,
  outdir,
  format: "iife",
  target: "chrome116",
  minify: true,
  sourcemap: false,
  logLevel: "info",
});
cpSync("static", outdir, { recursive: true });
const manifest = JSON.parse(readFileSync("static/manifest.json", "utf8"));
manifest.version = pkg.version;
if (store) {
  delete manifest.key;
}
writeFileSync(`${outdir}/manifest.json`, JSON.stringify(manifest, null, 2) + "\n");
