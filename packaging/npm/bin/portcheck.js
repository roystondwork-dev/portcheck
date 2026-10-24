#!/usr/bin/env node
// Runs portcheck with the given arguments: `npx portcheck-cli 3000`.
"use strict";

const { spawnSync } = require("node:child_process");
const { ensureBinary } = require("../lib/install.js");
const { version } = require("../package.json");
const checksums = require("../checksums.json");

ensureBinary({ version, checksums }).then(
  (bin) => {
    const r = spawnSync(bin, process.argv.slice(2), { stdio: "inherit" });
    if (r.error) {
      console.error(`portcheck-cli: couldn't run portcheck: ${r.error.message}`);
      process.exit(3);
    }
    if (r.signal) {
      process.kill(process.pid, r.signal); // e.g. Ctrl-C: exit the way portcheck did
    }
    process.exit(r.status ?? 1);
  },
  (err) => {
    console.error(`portcheck-cli: ${err.message}`);
    process.exit(3);
  },
);
