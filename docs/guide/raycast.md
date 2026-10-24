# Raycast

The portcheck extension for [Raycast](https://www.raycast.com) (macOS) puts your ports
one keystroke away.

| Command | What it does |
|---|---|
| **Search Ports** | Every port listening on your Mac with its process, PID, user and who started it. **⏎** stops it (after confirming), **⌘O** opens it in the browser, **⌘C** copies the PID; also copy the command line, or the stop command for a Docker port. |
| **Check Port** `3000` | Free, or which process holds it, who started it and its command; stop it from there. |
| **Kill Port** `3000` | Stops what listens on the port, after confirming. Never stops system processes. |
| **Find Free Port** `3000-3999` | Copies a free port, optionally in a range. |

## Install

!!! note "Status"
    The extension is on its way to the Raycast Store. Until then, build it from the
    repository: `cd extensions/raycast && npm ci && npm run dev`.

It needs the portcheck CLI ([install](../install.md)); the extension finds it in
Homebrew, `~/.local/bin` or `~/go/bin`, or set its path in the extension's preferences.
