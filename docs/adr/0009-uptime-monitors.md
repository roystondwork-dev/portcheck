# ADR-0009: Uptime monitors run on the user's device

- Status: accepted
- Date: 2026-11-11

## Context

Users asked for a way to know when a service they depend on goes down: a website, a
`/ping` or `/health` endpoint, a database port. An open port isn't enough (a server can
accept connections and still answer with errors), and checking by hand isn't monitoring.

Hosted uptime services check around the clock, but portcheck has no servers, and running
checks for every user would cost money (the project uses free services only) and send
users' URLs to a third party.

## Decision

- **Checks run on the user's device, only while it's on.** From
  `portcheck monitor run`, the tray app, or the VS Code extension while its Network Tools
  panel is open. The docs say plainly that checks stop when the device sleeps, and point
  to a scheduled GitHub Actions workflow for checks around the clock.
- **One list, shared by every front end.** Monitors live in `monitors.yaml` in
  portcheck's configuration folder (`$PORTCHECK_CONFIG_DIR`, like the port registry).
  `monitor add/remove` and the VS Code panel edit it; the tray app and `monitor run`
  reload it when it changes, so adding a monitor anywhere takes effect everywhere.
- **What "up" means is simple and explicit.** A URL is up when an HTTP GET answers within
  the timeout with a status in a configurable set (default `200-299`, after redirects)
  and, optionally, a body containing a given text. An address is up when it accepts a
  TCP connection. TLS certificates are verified; proxies come from the environment.
- **Few, meaningful alerts.** A monitor is down after 2 failed checks in a row
  (configurable), and the user hears about it once, and once more when it's back ("back
  up after 4m"). A cancelled check (shutdown) never counts as a failure.
- **Gentle by default.** Checks run every minute by default and never more often than
  every 10 seconds, with a 10-second timeout shorter than the interval.
- **Notifications with built-in tools.** osascript (macOS), a PowerShell toast (Windows)
  and notify-send (Linux), run directly with the text passed as arguments or environment
  variables, never interpolated into a script. No new dependencies.
- **The logic is a public, tested library.** `pkg/monitor` holds the file format,
  checks, the up/down state machine, the scheduler and the file-reloading supervisor;
  the CLI, the tray and the tests share it.

## Consequences

- No hosting, no cost, and URLs stay on the user's device; but no checks while it sleeps,
  and results show reachability from the user's network, not from the internet.
- Several front ends may run at once (tray and VS Code), and each alerts on its own; the
  VS Code panel checks only while it's open, which keeps duplicates rare.
- `monitor check` gives scripts and CI a one-shot check with an exit status.
