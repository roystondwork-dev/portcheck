# ADR-0005: A browser extension that talks to portcheck through native messaging

- **Status:** Accepted
- **Date:** 2026-10-20

## Context

Developers often meet a port problem in the browser first: `localhost:3000` shows
"This site can't be reached", or a page loads from a server they forgot was running.
The browser can't tell which process serves a port, or whether anything listens at
all. ADR-0004 left a browser extension as an optional later phase.

A browser extension can only reach a local program through **native messaging**: the
browser starts a program registered for the extension and exchanges JSON messages with
it over stdin/stdout, each prefixed with its length as a 32-bit integer in native byte
order. The program is registered with a small manifest file (a registry entry on
Windows) that lists which extension IDs may start it.

## Decision

1. **Chromium browsers first:** Chrome, Edge, Chromium and Brave, with Manifest V3.
   Firefox uses a different manifest format and can follow later.
2. **portcheck is the native host.** When a browser starts it with an extension origin
   (`chrome-extension://<id>/`) as its first argument, portcheck serves native messages
   instead of parsing a command line. No second program is installed.
3. **A fixed allowlist of operations:** `version`, `list` (listening sockets), `check`
   (one port), and `kill` (one PID). Each runs the matching portcheck command in-process
   with `--json` and returns its schema v1 output, so the extension sees exactly what
   the CLI prints, and `kill` keeps all its safety guards. Because the browser is the
   host's parent process, portcheck refuses to stop the browser itself. Commands that
   run programs or touch other hosts (`run`, `scan`, `wait`) are not reachable.
4. **`portcheck browser install`** writes the host manifest for the browsers it finds
   (per user, no administrator rights), allowing only portcheck's extension IDs;
   `uninstall` removes it and `status` reports it.
5. **The extension is a thin client** in `extensions/browser/`: strict TypeScript,
   esbuild, Vitest for logic, and an end-to-end test that loads it into a real Chromium
   with Playwright and checks the badge for a real local server. Its ID is pinned with
   the manifest `key`, so the host manifest can name it.

## Consequences

- ✅ One implementation of port logic and safety guards for the CLI, the TUI, VS Code
  and the browser.
- ✅ The browser can't use portcheck for anything beyond the four operations.
- ⚠️ Users run `portcheck browser install` once (the extension tells them to).
- ⚠️ Each message starts a short-lived portcheck process. That costs a few milliseconds
  on macOS (ADR-0003) and is only done for local tabs.
- ⚠️ Extension stores assign their own IDs; each store's ID has to be added to the
  allowed list when it's published there. The Chrome Web Store also charges a one-time
  developer fee.
