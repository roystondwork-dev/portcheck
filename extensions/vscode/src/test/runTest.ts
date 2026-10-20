// Launches VS Code with the extension under development and runs the suite.
// Needs a built portcheck binary: `make build` at the repository root, or PORTCHECK_BIN.
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";
import { runTests } from "@vscode/test-electron";

async function main(): Promise<void> {
  const root = path.resolve(__dirname, "..", "..");
  const exe = process.platform === "win32" ? "portcheck.exe" : "portcheck";
  const binary = process.env.PORTCHECK_BIN ?? path.join(root, "..", "..", "bin", exe);
  if (!fs.existsSync(binary)) {
    throw new Error(`portcheck binary not found at ${binary}; run "make build" in the repository root`);
  }
  const workspace = fs.mkdtempSync(path.join(os.tmpdir(), "portcheck-ws-"));
  fs.mkdirSync(path.join(workspace, ".vscode"));
  fs.writeFileSync(
    path.join(workspace, ".vscode", "settings.json"),
    JSON.stringify({ "portcheck.path": binary, "portcheck.refreshInterval": 0 }, null, 2),
  );
  await runTests({
    version: process.env.VSCODE_TEST_VERSION ?? "stable",
    extensionDevelopmentPath: root,
    extensionTestsPath: path.join(__dirname, "suite", "index.js"),
    launchArgs: [
      workspace,
      // VS Code's IPC socket lives in the user data dir; macOS limits socket
      // paths to 103 characters, so use a short temporary directory.
      `--user-data-dir=${fs.mkdtempSync(path.join(os.tmpdir(), "pcvs-"))}`,
      "--disable-extensions",
      "--disable-workspace-trust",
      "--skip-welcome",
      // Headless CI (Xvfb) has no GPU; VS Code's window sometimes hangs at
      // startup ("CodeWindow: detected unresponsive") while setting one up.
      "--disable-gpu",
    ],
    extensionTestsEnv: { PORTCHECK_TEST_BINARY: binary },
  });
}

main().catch((err: unknown) => {
  console.error(err);
  process.exit(1);
});
