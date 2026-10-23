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

## Amendment (2026-10-23): Firefox

Firefox followed in M18 with the same code and a separate build (`npm run
build:firefox`):

- Firefox starts a host with two arguments, the host manifest's path and the add-on
  ID, so portcheck also recognises that launch form. Its host manifests list add-on IDs
  in `allowed_extensions` and live in Mozilla's directory (registry key
  `HKCU\Software\Mozilla\NativeMessagingHosts` on Windows); `portcheck browser install`
  writes them for `--browser firefox`.
- The add-on ID is chosen by us (`portcheck@roystondwork-dev.github.io`) and kept by
  addons.mozilla.org, so the default registration allows the store version.
- Firefox runs Manifest V3 background scripts as an event page (no service worker),
  requires a data-collection declaration (none), and treats host permissions as
  optional; the popup asks for localhost access when it's missing.
- Firefox's reviewers rebuild bundled add-ons from source, so each release attaches the
  source with build instructions.
- Firefox only installs add-ons signed by Mozilla. Besides the Firefox Add-ons listing,
  each release has Mozilla sign a direct download (unlisted channel, minutes instead of
  a review queue). Mozilla signs a version number once per add-on ID, so the direct
  download is its own add-on, `portcheck-direct@roystondwork-dev.github.io`, which
  portcheck also allows by default. It updates itself from
  `docs/firefox-updates.json`, whose version release-please bumps.

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
