// Bundles the extension: background.js, popup.js and the static files, with
// the version from package.json in manifest.json.
//   node esbuild.mjs            → dist/         (release zip; the manifest key pins its ID)
//   node esbuild.mjs --store    → dist-store/   (Chrome/Edge stores; they assign the ID)
//   node esbuild.mjs --firefox  → dist-firefox/ (Firefox Add-ons; gecko ID, background scripts)
//   node esbuild.mjs --firefox-direct → dist-firefox-direct/ (the signed .xpi on GitHub
//                                releases: its own ID, updates from our update feed)
import { cpSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import * as esbuild from "esbuild";

const direct = process.argv.includes("--firefox-direct");
const target = direct || process.argv.includes("--firefox") ? "firefox" : process.argv.includes("--store") ? "store" : "release";
const outdir = direct ? "dist-firefox-direct" : { release: "dist", store: "dist-store", firefox: "dist-firefox" }[target];
const pkg = JSON.parse(readFileSync("package.json", "utf8"));

rmSync(outdir, { recursive: true, force: true });
await esbuild.build({
  entryPoints: ["src/background.ts", "src/popup.ts"],
  bundle: true,
  outdir,
  format: "iife",
  target: target === "firefox" ? "firefox128" : "chrome116",
  // Firefox's reviewers read the source; unminified output is easier to compare.
  minify: target !== "firefox",
  sourcemap: false,
  logLevel: "info",
});
cpSync("static", outdir, { recursive: true });
const manifest = JSON.parse(readFileSync("static/manifest.json", "utf8"));
manifest.version = pkg.version;
if (target !== "release") {
  delete manifest.key;
}
if (target === "firefox") {
  delete manifest.minimum_chrome_version;
  // Firefox runs background scripts as an event page, not a service worker.
  manifest.background = { scripts: ["background.js"] };
  manifest.browser_specific_settings = {
    gecko: {
      // Mozilla signs one version number once per ID, so the direct download is
      // a separate add-on from the Firefox Add-ons listing.
      id: direct ? "portcheck-direct@roystondwork-dev.github.io" : "portcheck@roystondwork-dev.github.io",
      strict_min_version: "140.0",
      // Firefox's built-in data consent (required for new add-ons): none collected.
      data_collection_permissions: { required: ["none"] },
      ...(direct && { update_url: "https://roystondwork-dev.github.io/portcheck/firefox-updates.json" }),
    },
  };
}
writeFileSync(`${outdir}/manifest.json`, JSON.stringify(manifest, null, 2) + "\n");
