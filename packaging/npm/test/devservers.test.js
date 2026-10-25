"use strict";

process.env.PORTCHECK_REGISTER_TEST = "1"; // load register.js without hooking this process

const assert = require("node:assert/strict");
const { EventEmitter } = require("node:events");
const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const net = require("node:net");
const os = require("node:os");
const path = require("node:path");
const { test } = require("node:test");
const check = require("../lib/check.js");
const { install } = require("../lib/register.js");
const vite = require("../lib/vite.js");
const { version } = require("../package.json");

const node = {
  pid: 42,
  process: {
    name: "node",
    ancestors: [
      { pid: 7, name: "npm" },
      { pid: 3, name: "zsh" },
    ],
  },
};
const inUse = (port, holders) => ({ ports: [{ port, proto: "tcp", status: "in-use", holders }] });

test("parses check output and nothing else", () => {
  const data = inUse(3000, [node]);
  assert.deepEqual(check.parse(JSON.stringify({ schemaVersion: 1, kind: "check", data })), data);
  assert.equal(check.parse(JSON.stringify({ kind: "list", data: { sockets: [] } })), null);
  assert.equal(check.parse(JSON.stringify({ kind: "check", data: {} })), null);
  assert.equal(check.parse("null"), null);
  assert.equal(check.parse("not json"), null);
});

test("describes who holds a port", () => {
  const ipv6 = { ...node, localAddress: "::" };
  assert.deepEqual(check.describe(3000, inUse(3000, [node, ipv6])), {
    lines: [
      "port 3000 is in use by node (pid 42), started by npm (7) ← zsh (3).",
      "Stop it: npx portcheck-cli kill 3000",
    ],
    targets: [{ pid: 42, name: "node" }],
  });
  assert.deepEqual(
    check.describe(3000, inUse(3000, [{ pid: 9 }])).lines[0],
    "port 3000 is in use by a process (pid 9).",
  );

  const web = { pid: 5, container: { runtime: "docker", name: "web", stopHint: "docker stop web" } };
  const byImage = { pid: 6, container: { runtime: "podman", image: "nginx", stopHint: "podman stop x" } };
  const byId = { container: { runtime: "docker", id: "abc", stopHint: "docker stop abc" } };
  const unnamed = { container: { runtime: "docker", stopHint: "docker stop ?" } };
  assert.deepEqual(check.describe(8080, inUse(8080, [web, byImage, byId, unnamed])), {
    lines: [
      'port 8080 is published by docker container "web". Stop it: docker stop web',
      'port 8080 is published by podman container "nginx". Stop it: podman stop x',
      'port 8080 is published by docker container "abc". Stop it: docker stop abc',
      'port 8080 is published by docker container "?". Stop it: docker stop ?',
    ],
    targets: [],
  });
  assert.deepEqual(check.describe(22, inUse(22, [{}])).lines, [
    "port 22 is held by another user's process. See it: sudo npx portcheck-cli 22",
  ]);
  assert.deepEqual(check.describe(22, inUse(22, [])).lines, [
    "port 22 is already in use. See what holds it: npx portcheck-cli 22",
  ]);
  assert.deepEqual(check.describe(22, { ports: [{ port: 22, status: "in-use" }] }).targets, []);
});

test("describes free, denied and unknown ports", () => {
  assert.match(check.describe(3000, { ports: [{ port: 3000, status: "free", holders: [] }] }).lines[0], /free now/);
  assert.match(
    check.describe(80, { ports: [{ port: 80, status: "denied", holders: [] }] }).lines[0],
    /permission denied/,
  );
  assert.deepEqual(check.describe(3000, null).lines, [
    "port 3000 is already in use. See what holds it: npx portcheck-cli 3000",
  ]);
  assert.deepEqual(check.describe(3000, { ports: [] }).targets, []);
});

test("finds portcheck without downloading", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "pc-bin-"));
  assert.equal(check.binarySync({ PORTCHECK_BIN: "/opt/pc" }, "linux", dir), "/opt/pc");
  assert.equal(check.binarySync({}, "linux", dir), "portcheck");
  assert.equal(check.binarySync({}, "win32", dir), "portcheck.exe");
  fs.mkdirSync(path.join(dir, version));
  fs.writeFileSync(path.join(dir, version, "portcheck"), "");
  assert.equal(check.binarySync({}, "linux", dir), path.join(dir, version, "portcheck"));
});

test("downloads portcheck only when it isn't there", async () => {
  assert.equal(await check.binary({ PORTCHECK_BIN: path.resolve("/opt/pc") }), path.resolve("/opt/pc"));
  const env = { PORTCHECK_CACHE: fs.mkdtempSync(path.join(os.tmpdir(), "pc-empty-")) };
  assert.equal(await check.binary(env, async () => "/downloaded"), "/downloaded");
  const fallback = process.platform === "win32" ? "portcheck.exe" : "portcheck";
  assert.equal(
    await check.binary(env, async () => {
      throw new Error("offline");
    }),
    fallback,
  );
});

test("runs check and kill, and reports failures as null or false", async () => {
  const out = JSON.stringify({ kind: "check", data: inUse(3000, [node]) });
  const calls = [];
  const sync = (bin, args, opts) => {
    calls.push([bin, args, opts.timeout]);
    return { status: 1, stdout: out };
  };
  assert.deepEqual(check.checkSync(3000, "/pc", sync), inUse(3000, [node]));
  assert.deepEqual(calls, [["/pc", ["check", "3000", "--json"], check.TIMEOUT_MS]]);
  assert.equal(
    check.checkSync(3000, "/pc", () => ({ error: new Error("ENOENT") })),
    null,
  );
  assert.equal(
    check.checkSync(3000, "/pc", () => ({ status: null, stdout: null })),
    null,
  );

  const exec =
    (stdout, err = null) =>
    (_bin, _args, _opts, cb) =>
      cb(err, stdout);
  assert.deepEqual(await check.check(3000, "/pc", exec(out, new Error("exit 1"))), inUse(3000, [node]));
  assert.equal(await check.check(3000, "/pc", exec(undefined, new Error("ENOENT"))), null);
  assert.equal(await check.kill(42, "/pc", exec("")), true);
  assert.equal(await check.kill(42, "/pc", exec("", new Error("exit 1"))), false);
});

/** A fresh Server class and process stand-in for each register test. */
function fakes() {
  class Server extends EventEmitter {}
  const proc = new EventEmitter();
  const out = [];
  return { Server, proc, out, write: (s) => out.push(s) };
}

const inUseError = (port = 3000) =>
  Object.assign(new Error(`listen EADDRINUSE: address already in use :::${port}`), {
    code: "EADDRINUSE",
    syscall: "listen",
    port,
  });

test("register: the error still reaches the program's own handler", () => {
  const f = fakes();
  const failed = install({ ...f, lookup: () => inUse(3000, [node]) });
  const s = new f.Server();
  const seen = [];
  s.on("error", (e) => seen.push(e));
  const err = inUseError();
  assert.equal(s.emit("error", err), true);
  assert.deepEqual(seen, [err]);
  assert.equal(failed.get(s), 3000);

  f.proc.emit("exit", 1);
  assert.deepEqual(f.out, [
    "portcheck: port 3000 is in use by node (pid 42), started by npm (7) ← zsh (3).\nportcheck: Stop it: npx portcheck-cli kill 3000\n",
  ]);
});

test("register: an unhandled error still throws, unchanged", () => {
  const f = fakes();
  install({ ...f, lookup: () => null });
  const err = inUseError(4000);
  assert.throws(
    () => new f.Server().emit("error", err),
    (e) => e === err,
  );
  f.proc.emit("exit", 1);
  assert.deepEqual(f.out, ["portcheck: port 4000 is already in use. See what holds it: npx portcheck-cli 4000\n"]);
});

test("register: quiet when the server recovers, the exit is clean, or it's another error", () => {
  const f = fakes();
  const lookups = [];
  const failed = install({ ...f, lookup: (p) => lookups.push(p) });
  const probe = new f.Server(); // a throwaway server that only checks the port
  probe.on("error", () => {});
  probe.emit("error", inUseError());
  new f.Server().emit("listening"); // the real one listens on the next port
  const other = new f.Server();
  other.on("error", () => {});
  other.emit("error", Object.assign(new Error("EACCES"), { code: "EACCES", syscall: "listen", port: 80 }));
  other.emit("error", Object.assign(new Error("pipe"), { code: "EADDRINUSE", syscall: "listen" })); // a socket path, no port
  other.emit("error", null);
  other.emit("data", "x");
  assert.equal(failed.size, 0);
  f.proc.emit("exit", 1);

  const g = fakes();
  install({ ...g, lookup: (p) => lookups.push(p) });
  const s = new g.Server();
  s.on("error", () => {});
  s.emit("error", inUseError());
  g.proc.emit("exit", 0);
  assert.deepEqual(lookups, []);
  assert.deepEqual([...f.out, ...g.out], []);
});

test("register: installs once, and a failing lookup never throws", () => {
  const f = fakes();
  const first = install({
    ...f,
    lookup: () => {
      throw new Error("boom");
    },
  });
  assert.equal(install({ ...f }), first);
  const s = new f.Server();
  s.on("error", () => {});
  s.emit("error", inUseError());
  assert.doesNotThrow(() => f.proc.emit("exit", 1));
  assert.deepEqual(f.out, []);
  assert.equal(f.proc.listenerCount("exit"), 1);
});

test("register: a bookkeeping failure never breaks emit", () => {
  const f = fakes();
  install({ ...f });
  const hostile = {
    get code() {
      throw new Error("getter");
    },
  };
  const s = new f.Server();
  const seen = [];
  s.on("error", (e) => seen.push(e));
  s.emit("error", hostile);
  assert.deepEqual(seen, [hostile]);
});

test("register, for real: explains the crash and keeps it", { skip: process.platform === "win32" }, async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "pc-reg-"));
  const holder = net.createServer().listen(0, "127.0.0.1");
  await new Promise((r) => holder.once("listening", r));
  const port = holder.address().port;
  try {
    // A stand-in portcheck that answers like the real one.
    const fake = path.join(dir, "portcheck");
    const answer = JSON.stringify({ schemaVersion: 1, kind: "check", data: inUse(port, [node]) });
    fs.writeFileSync(fake, `#!/bin/sh\necho '${answer}'\nexit 1\n`, { mode: 0o755 });
    const app = path.join(dir, "app.js");
    fs.writeFileSync(app, `require("node:http").createServer().listen(${port}, "127.0.0.1");\n`);
    const env = { ...process.env, PORTCHECK_BIN: fake };
    delete env.PORTCHECK_REGISTER_TEST;
    const r = spawnSync(process.execPath, ["-r", path.join(__dirname, "..", "lib", "register.js"), app], {
      env,
      encoding: "utf8",
    });
    assert.equal(r.status, 1);
    assert.match(r.stderr, /Error: listen EADDRINUSE: address already in use 127\.0\.0\.1:\d+/);
    assert.match(r.stderr, new RegExp(`portcheck: port ${port} is in use by node \\(pid 42\\)`));
    assert.match(r.stderr, new RegExp(`portcheck: Stop it: npx portcheck-cli kill ${port}`));
    // Node still reports the unhandled error itself.
    assert.match(r.stderr, /Unhandled 'error' event/);

    // Without portcheck at all: still a crash, plus a hint, and no hang.
    const none = spawnSync(process.execPath, ["-r", path.join(__dirname, "..", "lib", "register.js"), app], {
      env: { ...env, PORTCHECK_BIN: path.join(dir, "missing") },
      encoding: "utf8",
    });
    assert.equal(none.status, 1);
    assert.match(none.stderr, new RegExp(`portcheck: port ${port} is already in use. See what holds it`));
  } finally {
    holder.close();
  }
});

/** A Vite dev server stand-in. */
function fakeServer({ wanted = 5173, actual = 5174, address } = {}) {
  const logs = [];
  const httpServer = new EventEmitter();
  httpServer.address = () => (address !== undefined ? address : { port: actual });
  const server = {
    httpServer,
    logs,
    shortcuts: [],
    restarts: 0,
    _shortcutsState: undefined, // Vite 7.3+
    _currentServerPort: actual,
    config: {
      server: { port: wanted },
      logger: {
        info: (m) => logs.push(["info", m]),
        warn: (m) => logs.push(["warn", m]),
        error: (m) => logs.push(["error", m]),
      },
    },
    bindCLIShortcuts(opts) {
      server.shortcuts.push(...opts.customShortcuts);
    },
    async restart() {
      server.restarts++;
    },
  };
  return server;
}

function fakeDeps({ answers = [], killOk = true } = {}) {
  const calls = [];
  return {
    calls,
    binary: async () => "/pc",
    check: async (port, bin) => {
      calls.push(["check", port, bin]);
      return answers.length > 1 ? answers.shift() : answers[0];
    },
    describe: check.describe,
    kill: async (pid, bin) => {
      calls.push(["kill", pid, bin]);
      return killOk;
    },
  };
}

test("vite: quiet when Vite got the port it asked for", async () => {
  const deps = fakeDeps();
  await vite.onListening(fakeServer({ actual: 5173 }), deps);
  await vite.onListening(fakeServer({ address: null }), deps);
  await vite.onListening(fakeServer({ address: "/tmp/sock" }), deps);
  const noPort = fakeServer({ actual: 5173 });
  delete noPort.config.server.port; // Vite's default
  await vite.onListening(noPort, deps);
  assert.deepEqual(deps.calls, []);
});

test("vite: says who took the port and adds a k shortcut that stops it and restarts", async () => {
  const s = fakeServer();
  const deps = fakeDeps({ answers: [inUse(5173, [node])] });
  await vite.onListening(s, deps);
  assert.deepEqual(s.logs, [
    ["warn", "  portcheck: port 5173 is in use by node (pid 42), started by npm (7) ← zsh (3)."],
    ["warn", "  portcheck: Stop it: npx portcheck-cli kill 5173"],
    ["warn", "  portcheck: press k + enter to stop node and restart on port 5173"],
  ]);
  assert.equal(s.shortcuts.length, 1);
  assert.equal(s.shortcuts[0].key, "k");
  assert.match(s.shortcuts[0].description, /stop node \(pid 42\) and restart on port 5173/);

  await s.shortcuts[0].action(s);
  assert.equal(s._currentServerPort, undefined, "restart goes back to the configured port");
  assert.deepEqual(deps.calls.slice(1), [
    ["check", 5173, "/pc"],
    ["kill", 42, "/pc"],
  ]);
  assert.equal(s.restarts, 1);
  assert.match(s.logs.at(-1)[1], /stopped node \(pid 42\), restarting on port 5173/);
});

test("vite: on Vite before 7.3, only the explanation", async () => {
  const s = fakeServer();
  delete s._shortcutsState;
  await vite.onListening(s, fakeDeps({ answers: [inUse(5173, [node])] }));
  assert.equal(s.shortcuts.length, 0);
  assert.deepEqual(s.logs.at(-1), ["warn", "  portcheck: Stop it: npx portcheck-cli kill 5173"]);

  const old = fakeServer();
  delete old._currentServerPort;
  await vite.stopAndRestart(old, 5173, { pid: 42, name: "node" }, fakeDeps({ answers: [inUse(5173, [node])] }));
  assert.equal(old.restarts, 1);
});

test("vite: no shortcut for containers or several processes", async () => {
  for (const holders of [
    [{ pid: 5, container: { runtime: "docker", name: "web", stopHint: "docker stop web" } }],
    [node, { ...node, pid: 43 }],
  ]) {
    const s = fakeServer();
    await vite.onListening(s, fakeDeps({ answers: [inUse(5173, holders)] }));
    assert.equal(s.shortcuts.length, 0);
    assert.ok(s.logs.length > 0);
  }
});

test("vite: the shortcut re-checks before stopping anything", async () => {
  const target = { pid: 42, name: "node" };
  const free = { ports: [{ port: 5173, status: "free", holders: [] }] };

  let s = fakeServer();
  let deps = fakeDeps({ answers: [free] });
  await vite.stopAndRestart(s, 5173, target, deps);
  assert.equal(s.restarts, 1);
  assert.ok(!deps.calls.some((c) => c[0] === "kill"));
  assert.match(s.logs.at(-1)[1], /free now/);

  s = fakeServer();
  deps = fakeDeps({ answers: [inUse(5173, [{ ...node, pid: 99 }])] });
  await vite.stopAndRestart(s, 5173, target, deps);
  assert.equal(s.restarts, 0);
  assert.ok(!deps.calls.some((c) => c[0] === "kill"));
  assert.match(s.logs.at(-1)[1], /held by something else/);

  s = fakeServer();
  deps = fakeDeps({ answers: [inUse(5173, [node])], killOk: false });
  await vite.stopAndRestart(s, 5173, target, deps);
  assert.equal(s.restarts, 0);
  assert.deepEqual(s.logs.at(-1)[0], "error");
});

test("vite: the plugin waits for the server to listen and never throws", async () => {
  const plugin = vite();
  assert.equal(plugin.name, "portcheck");
  assert.equal(plugin.apply, "serve");
  assert.equal(vite.default, vite);
  assert.equal(vite.portcheck, vite);
  plugin.configureServer({ httpServer: null }); // middleware mode

  const s = fakeServer();
  const deps = fakeDeps({ answers: [inUse(5173, [node])] });
  vite({ deps }).configureServer(s);
  s.config.server.port = 5174; // Vite records the port it ended up on
  s.httpServer.emit("listening");
  await new Promise((r) => setImmediate(r));
  assert.equal(s.shortcuts.length, 1);

  const defaultPort = fakeServer();
  delete defaultPort.config.server.port;
  vite({ deps }).configureServer(defaultPort);
  defaultPort.httpServer.emit("listening");
  await new Promise((r) => setImmediate(r));
  assert.ok(defaultPort.logs.some((l) => l[1].includes("port 5173")));

  const broken = fakeServer();
  vite({
    deps: {
      ...deps,
      binary: async () => {
        throw new Error("boom");
      },
    },
  }).configureServer(broken);
  broken.httpServer.emit("listening");
  await new Promise((r) => setImmediate(r));
  assert.deepEqual(broken.logs, []);
});

test("the package's entry points load", () => {
  const pkg = require("../package.json");
  for (const entry of [".", "./vite"]) {
    const file = path.join(__dirname, "..", pkg.exports[entry].default);
    assert.ok(fs.existsSync(file), file);
    assert.ok(fs.existsSync(path.join(__dirname, "..", pkg.exports[entry].types)));
  }
  assert.ok(fs.existsSync(path.join(__dirname, "..", pkg.exports["./register"])));
  const lib = require("../lib/index.js");
  assert.deepEqual(Object.keys(lib).sort(), ["binary", "check", "describe", "ensureBinary", "kill"]);
});
