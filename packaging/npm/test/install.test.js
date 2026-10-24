"use strict";

const assert = require("node:assert/strict");
const { execFileSync } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { test } = require("node:test");
const { archiveName, cacheDir, ensureBinary, extract, sha256 } = require("../lib/install.js");

test("archive names match the release", () => {
  assert.equal(archiveName("0.9.0", "darwin", "arm64"), "portcheck_0.9.0_darwin_arm64.tar.gz");
  assert.equal(archiveName("0.9.0", "linux", "x64"), "portcheck_0.9.0_linux_amd64.tar.gz");
  assert.equal(archiveName("0.9.0", "win32", "arm64"), "portcheck_0.9.0_windows_arm64.zip");
  assert.throws(() => archiveName("0.9.0", "freebsd", "x64"), /isn't available for freebsd\/x64/);
  assert.throws(() => archiveName("0.9.0", "linux", "ia32"), /linux\/ia32/);
});

test("cache directory per OS, overridable", () => {
  assert.equal(cacheDir({ PORTCHECK_CACHE: "/c" }, "linux", "/h"), "/c");
  assert.equal(cacheDir({}, "darwin", "/h"), path.join("/h", "Library", "Caches", "port-check"));
  assert.equal(cacheDir({}, "linux", "/h"), path.join("/h", ".cache", "port-check"));
  assert.equal(cacheDir({ XDG_CACHE_HOME: "/x" }, "linux", "/h"), path.join("/x", "port-check"));
  assert.equal(cacheDir({ LOCALAPPDATA: "C:\\L" }, "win32", "/h"), path.join("C:\\L", "port-check"));
  assert.equal(cacheDir({}, "win32", "/h"), path.join("/h", "AppData", "Local", "port-check"));
});

/** A release archive holding a fake portcheck script, as GoReleaser lays it out. */
function fakeArchive(dir) {
  const src = fs.mkdtempSync(path.join(dir, "src-"));
  fs.writeFileSync(path.join(src, "portcheck"), "#!/bin/sh\necho fake portcheck \"$@\"\n");
  fs.writeFileSync(path.join(src, "README.md"), "readme");
  const name = archiveName("9.9.9", process.platform === "win32" ? "linux" : process.platform, process.arch);
  const file = path.join(dir, name);
  execFileSync("tar", ["-czf", file, "-C", src, "portcheck", "README.md"]);
  return { name, file, bytes: fs.readFileSync(file) };
}

function fakeFetch(bytes, calls) {
  return async (url) => {
    calls.push(url);
    return { ok: true, status: 200, arrayBuffer: async () => bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.length) };
  };
}

test("downloads, verifies, extracts and caches the binary", { skip: process.platform === "win32" }, async () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "pc-npm-"));
  const { name, file, bytes } = fakeArchive(tmp);
  const calls = [];
  const opts = { version: "9.9.9", checksums: { [name]: sha256(file) }, dir: path.join(tmp, "cache"), fetchFn: fakeFetch(bytes, calls), baseUrl: "https://x", log: () => {} };
  const bin = await ensureBinary(opts);
  assert.equal(bin, path.join(tmp, "cache", "9.9.9", "portcheck"));
  assert.equal(execFileSync(bin, ["3000"], { encoding: "utf8" }), "fake portcheck 3000\n");
  assert.deepEqual(calls, [`https://x/${name}`]);
  assert.equal(await ensureBinary(opts), bin); // cached: no second download
  assert.equal(calls.length, 1);
  assert.deepEqual(fs.readdirSync(path.join(tmp, "cache")), ["9.9.9"]); // no temp dirs left
});

test("refuses an archive that doesn't match its checksum", async () => {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "pc-npm-"));
  const name = archiveName("9.9.9", "linux", "x64");
  const opts = { version: "9.9.9", platform: "linux", arch: "x64", checksums: { [name]: "0".repeat(64) }, dir: path.join(tmp, "cache"), fetchFn: fakeFetch(Buffer.from("tampered"), []), baseUrl: "https://x", log: () => {} };
  await assert.rejects(ensureBinary(opts), /checksum mismatch .* not using it/);
  assert.deepEqual(fs.readdirSync(path.join(tmp, "cache")), []);
});

test("reports missing checksums and failed downloads", async () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "pc-npm-"));
  const base = { version: "9.9.9", platform: "linux", arch: "x64", dir, baseUrl: "https://x", log: () => {} };
  await assert.rejects(ensureBinary({ ...base, checksums: {} }), /no checksum for portcheck_9.9.9_linux_amd64.tar.gz/);
  const name = archiveName("9.9.9", "linux", "x64");
  await assert.rejects(
    ensureBinary({ ...base, checksums: { [name]: "x" }, fetchFn: async () => ({ ok: false, status: 404 }) }),
    /download failed: HTTP 404/,
  );
});

// The real thing: PORT_CHECK_E2E=1 downloads the published release.
test("runs the real portcheck from GitHub", { skip: !process.env.PORT_CHECK_E2E }, () => {
  const cache = fs.mkdtempSync(path.join(os.tmpdir(), "pc-npm-e2e-"));
  const out = execFileSync(process.execPath, [path.join(__dirname, "..", "bin", "portcheck.js"), "version"], {
    encoding: "utf8",
    env: { ...process.env, PORTCHECK_CACHE: cache },
  });
  assert.match(out, new RegExp(`^portcheck ${require("../package.json").version} `));
});

test("extracts with the right tool on each OS", () => {
  const calls = [];
  const run = (file, args) => calls.push([file, ...args]);
  extract("a.tar.gz", "d", "portcheck", "linux", {}, run);
  const sysroot = fs.mkdtempSync(path.join(os.tmpdir(), "pc-sysroot-"));
  extract("a.zip", "d", "portcheck.exe", "win32", { SystemRoot: sysroot }, run); // no tar.exe there
  fs.mkdirSync(path.join(sysroot, "System32"));
  fs.writeFileSync(path.join(sysroot, "System32", "tar.exe"), "");
  extract("a.zip", "d", "portcheck.exe", "win32", { SystemRoot: sysroot }, run);
  assert.deepEqual(calls, [
    ["tar", "-xf", "a.tar.gz", "-C", "d", "portcheck"],
    ["powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Expand-Archive -LiteralPath 'a.zip' -DestinationPath 'd' -Force"],
    [path.join(sysroot, "System32", "tar.exe"), "-xf", "a.zip", "-C", "d", "portcheck.exe"],
  ]);
});
