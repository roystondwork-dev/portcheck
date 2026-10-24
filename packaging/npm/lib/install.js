// Finds portcheck for this OS and CPU, downloading it from the GitHub release
// on first use. The archive is checked against the checksums published with
// this package (checksums.json, from the release's signed checksums.txt)
// before anything is extracted, then cached per version.
"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { execFileSync } = require("node:child_process");

const REPO = "roystondwork-dev/portcheck";

/** The release archive for a platform, e.g. portcheck_0.9.0_darwin_arm64.tar.gz. */
function archiveName(version, platform, arch) {
  const goos = { darwin: "darwin", linux: "linux", win32: "windows" }[platform];
  const goarch = { x64: "amd64", arm64: "arm64" }[arch];
  if (!goos || !goarch) {
    throw new Error(`portcheck isn't available for ${platform}/${arch} (macOS, Linux and Windows on x64 or arm64 are)`);
  }
  return `portcheck_${version}_${goos}_${goarch}.${goos === "windows" ? "zip" : "tar.gz"}`;
}

/** Where downloaded binaries are kept: PORTCHECK_CACHE, else the OS's cache directory. */
function cacheDir(env = process.env, platform = process.platform, home = os.homedir()) {
  if (env.PORTCHECK_CACHE) {
    return env.PORTCHECK_CACHE;
  }
  if (platform === "win32") {
    return path.join(env.LOCALAPPDATA || path.join(home, "AppData", "Local"), "port-check");
  }
  if (platform === "darwin") {
    return path.join(home, "Library", "Caches", "port-check");
  }
  return path.join(env.XDG_CACHE_HOME || path.join(home, ".cache"), "port-check");
}

function sha256(file) {
  return crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
}

/**
 * Extracts one file from a release archive. On Windows, the system's bsdtar
 * (Windows 10+) reads the .zip; the tar on PATH may be Git Bash's GNU tar,
 * which can't, so it isn't used. PowerShell is the fallback.
 */
function extract(archive, dir, file, platform = process.platform, env = process.env, run = execFileSync) {
  if (platform !== "win32") {
    run("tar", ["-xf", archive, "-C", dir, file], { stdio: "ignore" });
    return;
  }
  const tar = path.join(env.SystemRoot || "C:\\Windows", "System32", "tar.exe");
  if (fs.existsSync(tar)) {
    run(tar, ["-xf", archive, "-C", dir, file], { stdio: "ignore" });
    return;
  }
  const q = (s) => "'" + s.replace(/'/g, "''") + "'";
  run("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", `Expand-Archive -LiteralPath ${q(archive)} -DestinationPath ${q(dir)} -Force`], {
    stdio: "ignore",
  });
}

/**
 * Returns the path of the portcheck binary, downloading it if needed.
 * fetchFn and log are injectable for tests.
 */
async function ensureBinary({
  version,
  checksums,
  platform = process.platform,
  arch = process.arch,
  dir = cacheDir(),
  fetchFn = fetch,
  baseUrl = `https://github.com/${REPO}/releases/download/v${version}`,
  log = (m) => process.stderr.write(m + "\n"),
}) {
  const exe = platform === "win32" ? "portcheck.exe" : "portcheck";
  const target = path.join(dir, version, exe);
  if (fs.existsSync(target)) {
    return target;
  }
  const name = archiveName(version, platform, arch);
  const want = checksums[name];
  if (!want) {
    throw new Error(`no checksum for ${name}; reinstall port-check`);
  }
  log(`port-check: downloading portcheck ${version} (${name})...`);
  const res = await fetchFn(`${baseUrl}/${name}`);
  if (!res.ok) {
    throw new Error(`download failed: HTTP ${res.status} for ${baseUrl}/${name}`);
  }
  fs.mkdirSync(dir, { recursive: true });
  const work = fs.mkdtempSync(path.join(dir, ".tmp-"));
  try {
    const archive = path.join(work, name);
    fs.writeFileSync(archive, Buffer.from(await res.arrayBuffer()));
    const got = sha256(archive);
    if (got !== want) {
      throw new Error(`checksum mismatch for ${name} (expected ${want}, got ${got}); not using it`);
    }
    extract(archive, work, exe, platform);
    fs.chmodSync(path.join(work, exe), 0o755);
    fs.mkdirSync(path.dirname(target), { recursive: true });
    // Atomic, so a concurrent run never sees a half-written binary.
    fs.renameSync(path.join(work, exe), target);
  } finally {
    fs.rmSync(work, { recursive: true, force: true });
  }
  return target;
}

module.exports = { archiveName, cacheDir, ensureBinary, extract, sha256 };
