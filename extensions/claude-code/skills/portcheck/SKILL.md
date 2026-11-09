---
name: portcheck
description: >-
  Diagnoses and fixes "port already in use" errors (EADDRINUSE, "address already in
  use", "Port 3000 is in use", WinError 10048) with portcheck. Shows which process holds
  a port, who started it and which project it belongs to, then frees the port safely or
  picks a free one. Use when a dev server, database, test or container fails to start
  because its port is taken, when the user asks what is running on a port, or asks for a
  free port.
when_to_use: >-
  "port 3000 is already in use", "EADDRINUSE", "address already in use", "what's running
  on port 8080", "kill the process on port 5173", "free up port 5432", "find me a free
  port", "which dev servers are still running"
---

# Fixing "port already in use" with portcheck

portcheck answers "what holds this port?" the same way on macOS, Linux and Windows, and
stops processes with safety guards: it never stops system processes, the user's shell or
you, and it names the container instead of killing Docker's proxy.

## Which tools to use

Prefer portcheck's MCP tools when they are available (this plugin starts them):

| Tool | Use it to |
|---|---|
| `check_port` | see whether ports are free and, for each one in use, the process, PID, user, command line, who started it and the container |
| `list_ports` | list what is listening, optionally filtered by port range or process name |
| `find_free_port` | get a free port, optionally in a range such as `3000-3999` |
| `stop_process` | stop what listens on a port, or a PID; `dry_run` shows what it would stop |
| `wait_for_port` | wait until a server accepts connections, or until a port is free |

Without the MCP tools, run the CLI. `npx -y portcheck-cli` works with nothing installed;
use `portcheck` instead if it is on PATH. Add `--json` when you need to parse the result.

```sh
npx -y portcheck-cli 3000            # free? if not, who holds it (exit 1 when in use)
npx -y portcheck-cli kill 3000 --dry-run
npx -y portcheck-cli kill 3000 --yes # only after the user agreed
npx -y portcheck-cli free 3000-3999  # a free port
npx -y portcheck-cli servers         # the user's dev servers, with project and uptime
```

## How to handle a port conflict

1. **Find the port.** Read it from the error (`:::3000`, `port 5173`, `bind(2) for ...
   port 8080`). Python's `[Errno 48] Address already in use` names no port: take it from
   the command that was run (`runserver 8000`, `--port 5000`, Flask's default 5000).
2. **Look before acting.** Call `check_port` (or `portcheck <port> --json`) and tell the
   user what holds the port in one line: the process, its PID, who started it and its
   project, e.g. "port 3000 is held by node (pid 41852), started by npm in my-shop, two
   days old".
3. **Decide with the user.** Stopping a process is their call. Offer the choices that fit:
   - it is their own old dev server: stop it (`stop_process`), then rerun their command;
   - they want both running: start theirs on another port from `find_free_port`, passing it
     the tool's way (`--port`, `PORT=`, `-p`);
   - it is a container: suggest the stop command portcheck gives (such as
     `docker stop <name>`); don't stop the proxy process;
   - it is a system service or another user's process: explain, and suggest stopping the
     service (`brew services stop`, `systemctl stop`) or a different port. Never pass
     `--allow-system`.
4. **Stop, then verify.** After `stop_process`, check the port again (or `wait_for_port`
   with `free: true`) before rerunning the user's command.

## Things to know

- **macOS port 5000 and 7000** belong to AirPlay Receiver (`ControlCenter`). It restarts if
  stopped: suggest turning it off in System Settings → General → AirDrop & Handoff, or
  another port.
- **A hidden owner** (no PID) means another user or root holds the port. portcheck says so
  and suggests `sudo`; don't guess the process.
- **Windows `denied`** with nothing listening usually means a port reserved by Hyper-V, WSL
  or Docker (`netsh interface ipv4 show excludedportrange protocol=tcp`).
- **Forgotten servers:** when several projects fight over ports, `portcheck servers
  --older-than 1d` lists the user's old dev servers; offer `--stop` only if they ask.
- **Avoiding it next time:** `portcheck run --port 3000 --next -- npm run dev` starts on
  3000, or the next free port, and passes it in `$PORT`.

Docs: https://roystondwork-dev.github.io/portcheck/ · fixes for specific errors:
https://roystondwork-dev.github.io/portcheck/fix/
