---
title: "portcheck vs kill-port: a safer way to free a port"
description: "How portcheck compares with the kill-port npm package: see what you're stopping and who started it, a graceful stop before force, and protection for system processes."
---

# portcheck vs kill-port

[`kill-port`](https://www.npmjs.com/package/kill-port) is the most common answer to
"port 3000 is already in use", and it works. portcheck does the same job, but shows you
what you're about to stop first. Both run with `npx`:

```sh
npx kill-port 3000          # kill-port
npx portcheck-cli kill 3000 # portcheck
```

## What's different

| | kill-port 2.0 | portcheck |
|---|---|---|
| Shows what it's stopping before it does | no | yes: process, PID, user, and who started it |
| Asks before stopping | no | yes (`--yes` to skip) |
| How it stops | force right away (`kill -9`, `TaskKill /F`) | asks the program to exit, forces only after 5 s |
| Protects system processes | no | refuses `launchd`, `systemd`, `svchost.exe` and others |
| Container ports | kills whatever owns the port | names the container: `docker stop web` |
| Needs `lsof` / `netstat` | yes | no, it reads the OS directly |
| Just look, don't kill | no | `portcheck 3000` |
| Several ports | yes (`3000,3001`) | yes, and ranges (`3000-3010`) |
| Exit status for scripts | always 0, even when nothing was killed | `1` if nothing was stopped; see [exit codes](../exit-codes.md) |
| Also | | free-port finder, interactive view, VS Code / JetBrains / browser extensions |

## Why the graceful stop matters

`kill -9` ends a program instantly. A dev server killed this way can leave behind a lock
file, a half-written cache, or a database connection that the next run trips over.
portcheck sends the polite signal (`SIGTERM`) first, so the program can clean up, and
forces it only if it hasn't exited after `--grace` (5 seconds by default). Pass
`--force` to skip straight to it.

## Why seeing it first matters

"Port 3000" is not always your dev server. It can be another project, a tool your
editor started, or a container. portcheck's confirmation shows exactly what will stop:

```console
$ portcheck kill 3000
About to terminate:
  PID 42379  node  (port 3000), started by npm (4200) ← zsh (900)
Proceed? [y/N]
```

## Switching

In scripts and `package.json`, replace:

```sh
kill-port 3000
```

with:

```sh
portcheck kill 3000 --yes
```

`--yes` is required when there's no terminal to ask in (CI, scripts). Add `--dry-run`
to preview. As a library, portcheck is a Go package, not a JavaScript one, so code that
calls `require("kill-port")` should keep using it, or run the `portcheck` command.

## Related

- [All "port already in use" fixes](index.md)
- [Stop a process (kill)](../guide/kill.md)
