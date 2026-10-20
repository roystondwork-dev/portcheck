---
title: portcheck
hide:
  - navigation
---

# portcheck

**See what's using your ports, find free ones, free them up, and check remote hosts.**
A single static binary for macOS, Linux and Windows.

![portcheck demo](assets/demo.gif)

```console
$ portcheck 3000
PORT  PROTO  STATUS  PID    PROCESS  USER
3000  tcp    in-use  42379  node     alice

$ portcheck kill 3000
About to terminate:
  PID 42379  node  (port 3000)
Proceed? [y/N] y
PID    PROCESS  RESULT      DETAIL
42379  node     terminated
```

<div class="grid cards" markdown>

-   :material-format-list-bulleted: **[Inspect](guide/list.md)**

    `portcheck list -l`: every listening socket with its process, PID and user.

-   :material-check-circle-outline: **[Check](guide/check.md)**

    `portcheck 5432`: is it free? If not, who holds it? Script-friendly exit codes.

-   :material-magnify: **[Find free ports](guide/free.md)**

    `portcheck free 3000-3999 -n 3`: free ports, verified by actually binding them.

-   :material-stop-circle-outline: **[Stop a process](guide/kill.md)**

    `portcheck kill 8080`: graceful first, force if needed, with guards against killing system processes.

-   :material-radar: **[Scan remote hosts](guide/scan.md)**

    `portcheck scan db.internal -p 5432`: rate-limited TCP connect checks, with no special privileges needed.

-   :material-timer-sand: **[Wait for ports](guide/wait.md)**

    `portcheck wait db:5432 -t 60s`: block until services are up, or until a port is free. Built for CI.

-   :material-play-circle-outline: **[Run on a free port](guide/run.md)**

    `portcheck run --port 3000 --next -- npm run dev`: start your dev server on a usable port, with `$PORT` set.

-   :material-monitor-dashboard: **[Interactive view](guide/tui.md)**

    `portcheck`: a live, sortable, filterable table. Stop processes without leaving it.

</div>

## Why portcheck?

- **One tool, every OS.** It replaces `lsof -i`, `netstat -ano`, `ss -ltnp`, `Get-NetTCPConnection` and `kill`, with the same flags and output on macOS, Linux and Windows.
- **Made for scripts.** `--json` output follows a [versioned schema](json-schema.md), and the [exit codes](exit-codes.md) mean the same thing on every command.
- **Container-aware.** Ports published by Docker or Podman show the container, not a proxy process. See [Containers](guide/containers.md).
- **Safe by default.** `kill` asks before it acts, refuses PID 1, itself, your shell and well-known system processes, and tries a graceful stop first. `scan` is rate-limited and has no stealth features.
- **Honest about permissions.** When the OS hides process owners, portcheck tells you instead of silently showing less. See [Permissions by OS](permissions.md).
- **Usable as a library.** Everything is built on importable Go packages. See the [Go library](library.md) page.

[Install portcheck](install.md){ .md-button .md-button--primary } [Quickstart](quickstart.md){ .md-button }
