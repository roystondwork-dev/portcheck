---
title: "OSError: [Errno 48] Address already in use (Python, Django, Flask)"
description: "Fix “Address already in use” in Python, Django runserver, Flask, FastAPI/uvicorn and http.server: find the process holding the port and stop it. Includes Flask's port 5000 and macOS AirPlay."
---

# Address already in use (Python, Django, Flask)

The same problem has a different message in each framework:

| You run | You see |
|---|---|
| any Python server on macOS | `OSError: [Errno 48] Address already in use` |
| any Python server on Linux | `OSError: [Errno 98] Address already in use` |
| any Python server on Windows | `[WinError 10048] Only one usage of each socket address ... is normally permitted` |
| `python manage.py runserver` | `Error: That port is already in use.` |
| `flask run` | `Address already in use` / `Port 5000 is in use by another program.` |
| `uvicorn main:app` | `[Errno 48] error while attempting to bind on address ('127.0.0.1', 8000): address already in use` |

Another process is listening on the port. Usually it's an earlier run of your own
server, still alive in another terminal or in the background.

## Fix it

```sh
npx portcheck-cli kill 8000
```

Or, without Node.js:

```sh
pipx install portcheck-tool
portcheck kill 8000
```

```console
About to terminate:
  PID 41852  Python  (port 8000), started by zsh (900)
Proceed? [y/N] y
PID    PROCESS  RESULT      DETAIL
41852  Python  terminated
```

portcheck shows the process and what started it before it asks, stops it gently first
so it can shut down cleanly, and never touches system processes.

## Port 5000 on macOS: AirPlay Receiver

On macOS 12 and later, Flask's default port 5000 (and port 7000) is taken by **AirPlay
Receiver**. `portcheck 5000` shows the holder as `ControlCenter`. Don't kill it: macOS
restarts it. Either:

- turn it off: **System Settings → General → AirDrop & Handoff → AirPlay Receiver**, or
- run Flask on another port: `flask run --port 5001`.

## Start on a free port instead

`portcheck run` picks a free port and puts it into your command with `{port}`:

```sh
portcheck run --port 8000 --next -- python manage.py runserver {port}
portcheck run --port 5000 --next -- flask run --port {port}
portcheck run --port 8000 --next -- uvicorn main:app --port {port}
```

Without `--next`, it stops with an explanation of who holds the port instead of a
traceback.

## Django's autoreloader

`runserver` runs your project in a child process that reloads on changes. If you
suspended it with ++ctrl+z++ or closed the terminal without stopping it, both the parent
and the child may still be running. `portcheck kill 8000` stops the one holding the
port; check `portcheck list --name 'python*'` for leftovers.

## Related

- [macOS](macos.md) · [Windows](windows.md) · [Linux](linux.md) · [Docker](docker.md)
- [All "port already in use" fixes](index.md)
