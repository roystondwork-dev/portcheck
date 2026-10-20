# Stop a process: `kill`

```console
$ portcheck kill 3000
About to terminate:
  PID 42379  node  (port 3000)
Proceed? [y/N] y
PID    PROCESS  RESULT      DETAIL
42379  node     terminated
```

`kill` finds the processes **listening** on the given ports (TCP and UDP by default),
asks for confirmation, and stops them.

## How a process is stopped

1. **Graceful:** portcheck sends `SIGTERM`, so the program can shut down cleanly.
2. **Wait:** it waits up to `--grace` (default `5s`) for the process to exit.
3. **Force:** if the process is still running, it sends `SIGKILL`.

`--force` (`-f`) skips straight to step 3.

!!! note "Windows"
    Windows has no general way to ask another process to shut down cleanly, so
    both steps use `TerminateProcess`.

The `RESULT` column says what happened:

| Result | Meaning |
|---|---|
| `terminated` | exited after the graceful request |
| `killed` | had to be force-killed |
| `already-exited` | was gone before portcheck could signal it |
| `dry-run` | `--dry-run`: nothing was sent |
| `failed` | see `DETAIL` |

## Safety

portcheck **never** kills:

- PID 0 or 1;
- itself, or its parent process (usually your shell).

It also refuses well-known system processes, such as `launchd`, `WindowServer`,
`systemd`, `init`, `lsass.exe`, `csrss.exe` and `svchost.exe`, and container/WSL
infrastructure such as `com.docker.backend`, `dockerd`, `containerd`, `docker-proxy`
and `wslrelay.exe`, unless you pass `--allow-system`.

Ports published by a container are never handled by killing the proxy: portcheck
names the container and suggests `docker stop <name>` instead. See
[Containers](containers.md#stopping-a-containers-port). These checks run **before** the confirmation prompt, so a
refusal never depends on what you answer.

## Flags

| Flag | Meaning |
|---|---|
| `--pid N` | also stop PID N (repeatable); works without a port |
| `--proto tcp` | only match ports on this protocol (default `tcp,udp`) |
| `-y`, `--yes` | don't ask for confirmation |
| `-f`, `--force` | skip the graceful step |
| `--grace 10s` | how long to wait before forcing |
| `--dry-run` | show what would happen; no confirmation needed |
| `--allow-system` | allow protected system processes (**dangerous**) |

## Non-interactive use

Without a terminal on stdin (CI, pipes, cron), `kill` refuses to run unless you pass `--yes`.
This protects you from piping a command into something that kills processes.

```sh
portcheck kill 8080 --yes --grace 10s
```

## Exit status

| Code | Meaning |
|---|---|
| `0` | every target stopped (or dry run) |
| `1` | nothing listening, a given PID doesn't exist, you declined, a protected process was refused, or the port belongs to a container |
| `2` | invalid arguments, or no `--yes` without a terminal |
| `3` | a process survived `SIGKILL`, or another error |
| `4` | permission denied (the port's owner is hidden, or you can't signal it) |

When the result is `4`, re-run with `sudo` or as Administrator; see [Permissions by OS](../permissions.md).
