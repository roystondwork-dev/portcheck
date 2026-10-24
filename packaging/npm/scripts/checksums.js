// Writes checksums.json from a release's checksums.txt (the archives only):
//   node scripts/checksums.js <version> <checksums.txt>
"use strict";

const fs = require("node:fs");
const path = require("node:path");

const [version, file] = process.argv.slice(2);
const out = {};
for (const line of fs.readFileSync(file, "utf8").split("\n")) {
  const [sum, name] = line.trim().split(/\s+/);
  if (name && name.startsWith(`portcheck_${version}_`) && /\.(tar\.gz|zip)$/.test(name)) {
    out[name] = sum;
  }
}
if (Object.keys(out).length !== 6) {
  console.error(`expected 6 archives for ${version} in ${file}, found ${Object.keys(out).length}`);
  process.exit(1);
}
fs.writeFileSync(path.join(__dirname, "..", "checksums.json"), JSON.stringify(out, null, 2) + "\n");
