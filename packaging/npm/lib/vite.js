// Vite plugin: `import portcheck from "portcheck-cli/vite"`, then
// `plugins: [portcheck()]`.
//
// When the port you asked for is taken, Vite quietly moves to the next one.
// This plugin then says who holds your port and adds a `k + enter` shortcut
// that stops that process (with portcheck kill's guards) and restarts Vite on
// the port you asked for (the shortcut needs Vite 7.3 or later).
"use strict";

const defaults = require("./check.js");

const DEFAULT_PORT = 5173;

/**
 * What to do once the dev server listens. wanted is the port asked for, read
 * before listening: Vite updates config.server.port to the port it ended up on.
 */
async function onListening(server, deps = defaults, wanted = server.config.server.port ?? DEFAULT_PORT) {
  const addr = server.httpServer && server.httpServer.address();
  if (!addr || typeof addr !== "object" || addr.port === wanted) {
    return;
  }
  const logger = server.config.logger;
  const bin = await deps.binary();
  const { lines, targets } = deps.describe(wanted, await deps.check(wanted, bin));
  for (const l of lines) {
    logger.warn(`  portcheck: ${l}`);
  }
  if (targets.length !== 1) {
    return; // nothing to stop, or several processes: leave the choice to the user
  }
  // Vite 7.3+ merges shortcuts from several bindCLIShortcuts calls (it keeps
  // them in _shortcutsState). Older versions would read the terminal twice,
  // so every shortcut, `r` included, would run twice: the kill command above
  // has to do there.
  if (!("_shortcutsState" in server)) {
    return;
  }
  const target = targets[0];
  logger.warn(`  portcheck: press k + enter to stop ${target.name} and restart on port ${wanted}`);
  server.bindCLIShortcuts({
    customShortcuts: [
      {
        key: "k",
        description: `stop ${target.name} (pid ${target.pid}) and restart on port ${wanted}`,
        action: (s) => stopAndRestart(s, wanted, target, deps),
      },
    ],
  });
}

/** The `k` shortcut: stop the holder if it's still the same process, then restart. */
async function stopAndRestart(server, port, target, deps = defaults) {
  const logger = server.config.logger;
  const bin = await deps.binary();
  // The process may have exited, or the PID been reused, since the warning.
  const now = deps.describe(port, await deps.check(port, bin)).targets;
  if (now.length === 0) {
    logger.info(`  portcheck: port ${port} is free now, restarting`);
  } else if (now.length !== 1 || now[0].pid !== target.pid) {
    logger.warn(`  portcheck: port ${port} is now held by something else; run npx portcheck-cli ${port}`);
    return;
  } else if (!(await deps.kill(target.pid, bin))) {
    logger.error(`  portcheck: couldn't stop ${target.name} (pid ${target.pid}); run npx portcheck-cli kill ${port}`);
    return;
  } else {
    logger.info(`  portcheck: stopped ${target.name} (pid ${target.pid}), restarting on port ${port}`);
  }
  // A restart keeps the port Vite moved to (it remembers it in
  // _currentServerPort, Vite 5 to 8). Forget it, so the restart binds the
  // configured port again; without the field, Vite restarts where it is.
  if ("_currentServerPort" in server) {
    server._currentServerPort = undefined;
  }
  await server.restart();
}

/** The plugin. options.deps replaces the portcheck calls (tests only). */
function portcheck(options = {}) {
  const deps = options.deps || defaults;
  return {
    name: "portcheck",
    apply: "serve",
    configureServer(server) {
      if (!server.httpServer) {
        return; // middleware mode: someone else owns the port
      }
      const wanted = server.config.server.port ?? DEFAULT_PORT;
      server.httpServer.once("listening", () => {
        onListening(server, deps, wanted).catch(() => {});
      });
    },
  };
}

module.exports = portcheck;
module.exports.default = portcheck;
module.exports.portcheck = portcheck;
module.exports.onListening = onListening;
module.exports.stopAndRestart = stopAndRestart;
