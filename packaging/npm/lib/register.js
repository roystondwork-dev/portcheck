// Preload hook: `node -r portcheck-cli/register app.js`, or
// NODE_OPTIONS=--require=portcheck-cli/register for any Node tool.
//
// When a server can't listen because its port is taken, and the program then
// exits with an error, it says who holds the port and how to stop it. The
// program itself behaves exactly as before: no 'error' listener is added
// (that would swallow the crash); Server#emit is wrapped to take note, and the
// explanation is printed from the process's 'exit' event.
//
// Once any server listens, earlier failures are forgotten: the program
// recovered (Vite, detect-port and others probe for a free port this way).
"use strict";

const net = require("node:net");
const { checkSync, describe } = require("./check.js");

const PATCHED = Symbol.for("portcheck-cli.register");

/**
 * Installs the hook on a Server class and process. Returns the record of
 * failed listens (server → port) so tests can inspect it.
 */
function install({
  Server = net.Server,
  proc = process,
  lookup = checkSync,
  write = (s) => proc.stderr.write(s),
} = {}) {
  if (Server.prototype[PATCHED]) {
    return Server.prototype[PATCHED];
  }
  const failed = new Map();
  const emit = Server.prototype.emit;
  Server.prototype.emit = function (event, err) {
    try {
      if (
        event === "error" &&
        err &&
        err.code === "EADDRINUSE" &&
        err.syscall === "listen" &&
        Number.isInteger(err.port) &&
        err.port > 0
      ) {
        failed.set(this, err.port);
      } else if (event === "listening") {
        // The program is serving after all: it handled the failure (Vite
        // probes ports with throwaway servers, then listens on a free one).
        failed.clear();
      }
    } catch {
      // Bookkeeping must never break the server.
    }
    return emit.apply(this, arguments);
  };
  Object.defineProperty(Server.prototype, PATCHED, { value: failed });

  proc.on("exit", (code) => {
    if (code === 0 || failed.size === 0) {
      return;
    }
    try {
      for (const port of new Set(failed.values())) {
        const { lines } = describe(port, lookup(port));
        write(lines.map((l) => `portcheck: ${l}\n`).join(""));
      }
    } catch {
      // Never turn one failure into two.
    }
  });
  return failed;
}

// Tests load this module without installing the hook into their own process.
if (!process.env.PORTCHECK_REGISTER_TEST) {
  install();
}

module.exports = { install };
