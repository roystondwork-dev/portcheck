// Asks portcheck who holds a port, for the dev-server hooks (register.js,
// vite.js). Everything here fails quietly: a missing or slow portcheck must
// never get in the way of the program it explains.
"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { execFile, spawnSync } = require("node:child_process");
const { cacheDir, ensureBinary } = require("./install.js");
const { version } = require("../package.json");
const checksums = require("../checksums.json");

/** How long a lookup may take before it's given up. */
const TIMEOUT_MS = 2000;

/**
 * The portcheck to run without downloading anything: PORTCHECK_BIN, else the
 * binary this package already cached, else `portcheck` on PATH.
 */
function binarySync(env = process.env, platform = process.platform, dir = cacheDir(env, platform)) {
  if (env.PORTCHECK_BIN) {
    return env.PORTCHECK_BIN;
  }
  const exe = platform === "win32" ? "portcheck.exe" : "portcheck";
  const cached = path.join(dir, version, exe);
  return fs.existsSync(cached) ? cached : exe;
}

/** The binary, downloading this package's version if needed (async callers only). */
async function binary(env = process.env, ensure = ensureBinary) {
  const b = binarySync(env);
  if (path.isAbsolute(b)) {
    return b;
  }
  try {
    return await ensure({ version, checksums, log: () => {} });
  } catch {
    return b;
  }
}

/** Parses `portcheck check --json` output; null if it isn't one. */
function parse(stdout) {
  try {
    const doc = JSON.parse(stdout);
    return doc && doc.kind === "check" && doc.data && Array.isArray(doc.data.ports) ? doc.data : null;
  } catch {
    return null;
  }
}

/** Runs `portcheck check <port> --json` and waits for it; null on any failure. */
function checkSync(port, bin = binarySync(), run = spawnSync) {
  const r = run(bin, ["check", String(port), "--json"], { encoding: "utf8", timeout: TIMEOUT_MS, windowsHide: true });
  return r.error ? null : parse(r.stdout || "");
}

/** The same, without blocking; resolves to null on any failure. */
function check(port, bin, run = execFile) {
  return new Promise((resolve) => {
    // check exits 1 when the port is in use; stdout still holds the answer.
    run(bin, ["check", String(port), "--json"], { timeout: TIMEOUT_MS, windowsHide: true }, (_err, stdout) => {
      resolve(parse(stdout || ""));
    });
  });
}

/** Stops one process with kill's guards; resolves to true if portcheck stopped it. */
function kill(pid, bin, run = execFile) {
  return new Promise((resolve) => {
    run(bin, ["kill", "--pid", String(pid), "--yes"], { timeout: 10000, windowsHide: true }, (err) => resolve(!err));
  });
}

function holderName(s) {
  if (s.container) {
    return `${s.container.runtime} container "${s.container.name || s.container.image || s.container.id || "?"}"`;
  }
  return s.process && s.process.name ? s.process.name : "a process";
}

/**
 * Says who holds a port, from a check's data:
 * { lines: [...], targets: [{ pid, name }] } where targets are the processes
 * that `portcheck kill` may stop (not containers or hidden owners).
 */
function describe(port, data) {
  const p = data && data.ports.find((x) => x.port === port);
  if (!p) {
    return { lines: [`port ${port} is already in use. See what holds it: npx portcheck-cli ${port}`], targets: [] };
  }
  if (p.status === "free") {
    return { lines: [`port ${port} was in use, and it's free now: try again.`], targets: [] };
  }
  if (p.status === "denied") {
    return { lines: [`port ${port} can't be used by your user (permission denied). Pick another port.`], targets: [] };
  }
  const lines = [];
  const targets = [];
  const seen = new Set();
  for (const s of p.holders || []) {
    const key = s.container ? `c:${s.container.stopHint}` : `p:${s.pid}`;
    if (seen.has(key)) {
      continue; // one line per process, even with IPv4 and IPv6 sockets
    }
    seen.add(key);
    if (s.container) {
      lines.push(`port ${port} is published by ${holderName(s)}. Stop it: ${s.container.stopHint}`);
    } else if (s.pid === undefined) {
      lines.push(`port ${port} is held by another user's process. See it: sudo npx portcheck-cli ${port}`);
    } else {
      const by = ((s.process && s.process.ancestors) || []).map((a) => `${a.name} (${a.pid})`).join(" ← ");
      lines.push(`port ${port} is in use by ${holderName(s)} (pid ${s.pid})${by ? `, started by ${by}` : ""}.`);
      targets.push({ pid: s.pid, name: holderName(s) });
    }
  }
  if (targets.length > 0) {
    lines.push(`Stop it: npx portcheck-cli kill ${port}`);
  }
  if (lines.length === 0) {
    lines.push(`port ${port} is already in use. See what holds it: npx portcheck-cli ${port}`);
  }
  return { lines, targets };
}

module.exports = { TIMEOUT_MS, binary, binarySync, check, checkSync, describe, kill, parse };
