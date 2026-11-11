# Uptime monitors: `monitor`

`portcheck monitor` keeps an eye on services you depend on, and warns you when one goes
down and when it comes back:

- **URLs** that must answer with a good HTTP status: a `/ping` or `/health` endpoint, a
  website, an API.
- **Ports** that must accept connections: a database, a cache, a dev server.

```console
$ portcheck monitor add https://example.com/health
Added example.com/health: checks https://example.com/health every 1m.
It's up now (84 ms).
To be warned when it goes down, keep 'portcheck monitor run' or the tray app running.

$ portcheck monitor check
NAME                STATUS  TIME    DETAIL
example.com/health  up      84 ms   HTTP 200
postgres            down    2 ms    connection refused
```

!!! note "Runs on your device, not in the cloud"
    Monitors run on **this device, while it's on**: in the [tray app](tray.md), in a
    terminal with `portcheck monitor run`, or in the [VS Code extension](vscode.md#network-tools)
    while its Network Tools panel is open. Nothing is hosted, nothing is sent anywhere
    except the checks themselves, and nothing costs money. The trade-off: checks stop when
    the device sleeps or shuts down. For a site that must be watched around the clock, see
    [Around the clock](#around-the-clock) below.

## Adding monitors

```sh
portcheck monitor add https://example.com/health                  # a URL
portcheck monitor add https://api.example.com/ping --contains pong  # the body must contain "pong"
portcheck monitor add db.internal:5432 --name postgres --every 5m   # a TCP port
portcheck monitor add 3000                                          # localhost:3000
```

| Flag | Default | Meaning |
|---|---|---|
| `--name` | the URL without `https://`, or the address | name in lists and alerts |
| `--every` | `1m` | how often to check; at least `10s` |
| `--timeout` | `10s` | how long one check may take; shorter than `--every` |
| `--status` | `200-299` | HTTP statuses that count as up, e.g. `200-399` or `200,204` (URLs only) |
| `--contains` | | text the response body must contain (URLs only) |

A URL is **up** when it answers within the timeout with a status in `--status` (after
following redirects) and, with `--contains`, a body containing that text. An address is
up when it accepts a TCP connection. `add` checks the new monitor once, so a typo shows
straight away.

`portcheck monitor list` shows the monitors; `portcheck monitor remove <name>` (or the URL
or address) removes one.

## Being warned

```console
$ portcheck monitor run
Monitoring 3 services (Ctrl-C to stop)…
2026-11-11 10:42:05  DOWN  api.example.com/ping is down: HTTP 503 Service Unavailable
2026-11-11 10:46:05  UP    api.example.com/ping is back up after 4m
```

`run` checks each monitor on its schedule and, for each change, prints a line and shows a
**desktop notification** (macOS Notification Center, Windows toast, Linux `notify-send`).
To avoid false alarms:

- a service counts as **down after 2 failed checks in a row** (`--threshold`), so one
  dropped request doesn't wake you;
- you get **one** notification when it goes down and one when it's back, not one per check.

Edits to the monitors file (from `monitor add` in another terminal, or by hand) are picked
up while `run` is running. `--no-notify` prints alerts without notifications.

!!! tip "On macOS: keep alerts on screen"
    macOS shows banners for about 5 seconds; missed ones stay in Notification Center
    (click the clock). portcheck's notifications are listed under **Script Editor** in
    **System Settings → Notifications**, because they're shown through macOS's built-in
    `osascript`. Set its alert style to **Persistent** (older macOS: **Alerts**) to keep
    them until you close them. On Linux, notifications need `notify-send` (package
    `libnotify-bin` or `libnotify`).

### In the background

`portcheck monitor run` stops when its terminal closes. For checks whenever you're logged
in, use the **[tray app](tray.md)**: it runs the same monitors in the background, shows
them in its menu (✓ up, ✗ down with the reason), puts the number of services down next to
its icon, and sends the same notifications. Start it at login to keep it going.

## Checking once

`portcheck monitor check` checks every monitor (or the named ones) once, in parallel, and
exits with `1` if any is down, so it fits scripts and CI:

```sh
portcheck monitor check || echo "something is down"
portcheck monitor check postgres --json
```

Here one failed check counts as down. With `--json` it prints a
[`monitorCheck`](../json-schema.md#monitorcheck) document.

## The monitors file

Monitors live in `monitors.yaml` in portcheck's configuration folder (or in
`$PORTCHECK_CONFIG_DIR`). The commands keep it up to date; you can also edit it:

```yaml
version: 1
monitors:
  - name: example.com/health
    url: https://example.com/health
  - name: api
    url: https://api.example.com/ping
    every: 30s
    contains: pong
  - name: postgres
    address: db.internal:5432
    every: 5m
```

| OS | Folder |
|---|---|
| macOS | `~/Library/Application Support/portcheck` |
| Linux | `$XDG_CONFIG_HOME/portcheck`, usually `~/.config/portcheck` |
| Windows | `%AppData%\portcheck` |

## Around the clock

A device that sleeps can't watch a site at night. If you need that, without paying for a
service, the [portcheck GitHub Action](github-action.md) can run on a schedule on GitHub's
servers; GitHub emails you when a run fails:

```yaml
# .github/workflows/uptime.yml
on:
  schedule:
    - cron: "*/15 * * * *" # every 15 minutes
  workflow_dispatch:
jobs:
  uptime:
    runs-on: ubuntu-latest
    steps:
      - uses: roystondwork-dev/portcheck@v0.14.0 # installs portcheck
      - run: |
          portcheck monitor add https://example.com/health --contains ok
          portcheck monitor check # exits 1, failing the run, if it's down
```

GitHub runs scheduled workflows on a best-effort basis (they can start late), and checks
from there show whether your site is reachable from the internet rather than from your
network.

## Good to know

- Checks are plain HTTP GET requests (with a `portcheck/<version>` user agent) or TCP
  connections, made from your device. Only monitor services you own or are allowed to
  check, and keep `--every` reasonable for services you don't run.
- HTTPS certificates are verified; an expired or wrong certificate counts as down
  ("TLS certificate problem").
- Proxies set in `HTTPS_PROXY`/`HTTP_PROXY` are used.
