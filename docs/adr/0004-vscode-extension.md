# ADR-0004: A VS Code extension as a thin client of the portcheck CLI

- **Status:** Accepted
- **Date:** 2026-10-20

## Context

Port problems show up while developers are in their editor: a dev server fails with
`EADDRINUSE: address already in use :::3000`, and fixing it means leaving the editor
for a terminal, finding the process, and deciding whether it's safe to stop.

Existing options:

- **VS Code's built-in Ports view** manages port *forwarding* (SSH, Codespaces,
  dev containers). It doesn't say which local process holds a port or help free it.
- **Marketplace extensions** such as Port Monitor show port status and kill processes,
  but don't explain who holds a port, have no safety guards, and don't understand
  containers or WSL.
- **A browser extension** could only see the symptom ("localhost refused to connect")
  and can reach a local program only through *native messaging*, which needs a
  separate per-browser manifest install (and registry entries on Windows).

portcheck already has the hard parts: process attribution, parent chains, containers,
WSL, safe kill, free-port finding, `.portcheck.yaml`, and a versioned JSON output.

## Decision

1. **Build a VS Code extension first.** A browser extension is an optional later phase.
2. **Keep the extension a thin client.** It runs the `portcheck` CLI with `--json`
   ([schema v1](../json-schema.md)) and never reimplements port logic. All behaviour,
   including safety guards, stays in Go and is tested once.
3. **Bundle the binary.** Publish one VSIX per platform (`darwin-arm64`, `darwin-x64`,
   `linux-x64`, `linux-arm64`, `win32-x64`, `win32-arm64`) containing that platform's
   `portcheck` from the same release. The setting `portcheck.path` and the `PATH` are
   fallbacks.
4. **Run where the code runs.** `extensionKind: ["workspace"]`, so in Remote-SSH, WSL
   and Dev Containers the extension and its `portcheck` run on the remote host and see
   its ports.
5. **The USP is "error to fix in one click".** "Port in use" errors printed in the
   integrated terminal become links that open a fix: stop the holder (with the CLI's
   guards and confirmation), run on the next free port, or the right `docker stop` /
   `wsl` command.
6. **Same repository and CI.** The extension lives in `extensions/vscode/`, uses strict
   TypeScript, bundles with esbuild, and is tested with Vitest (pure logic) and
   `@vscode/test-electron` (the real extension inside VS Code). It ships with every
   portcheck release, to the VS Code Marketplace and Open VSX (for VSCodium, Cursor
   and Windsurf).

## Consequences

- ✅ One implementation of port logic, so the extension can't drift from the CLI.
- ✅ Works the same locally and in remote setups.
- ✅ Every new CLI capability (e.g. a new container runtime) reaches the extension for free.
- ⚠️ Each refresh starts a `portcheck` process. On macOS that's now cheap (ADR-0003),
  and the view only refreshes while it's visible.
- ⚠️ The extension depends on the JSON schema. That's why the schema is versioned:
  `schemaVersion` is checked, and a mismatch is reported instead of misread.
- ⚠️ Marketplace and Open VSX publishing need publisher accounts that only the
  maintainer can create.
