---
title: "Address already in use in Rails, Spring Boot, Go, .NET and PHP"
description: "Fix port-in-use errors in Ruby on Rails (Puma), Java and Spring Boot, Go, ASP.NET Core (Kestrel) and PHP's built-in server: find the process holding the port and stop it."
---

# Address already in use: Rails, Spring Boot, Go, .NET, PHP

Whatever the language, the fix is the same: find what holds the port, then stop it or
use another port.

```sh
npx portcheck-cli kill 8080
```

portcheck shows the process and who started it, asks, stops it gently first, and never
touches system processes. Install it with `brew`, `npm`, `pipx` or Scoop to drop the
`npx` (see [Install](../install.md)).

## Ruby on Rails (Puma)

```text
Address already in use - bind(2) for "127.0.0.1" port 3000 (Errno::EADDRINUSE)
```

An old `rails server` is still running: `portcheck kill 3000`.

```text
A server is already running (pid: 41852, file: /app/tmp/pids/server.pid).
```

This one can be a **stale PID file** left by a crash. Check whether anything is really
on the port:

```sh
portcheck 3000
```

If it says the port is free, delete `tmp/pids/server.pid` and start again.

## Java and Spring Boot

```text
***************************
APPLICATION FAILED TO START
***************************

Description:

Web server failed to start. Port 8080 was already in use.
```

Plain Java reports `java.net.BindException: Address already in use`. Often the holder
is an earlier run from your IDE that is still alive: `portcheck kill 8080` shows it as
`java`, started by `idea` or your terminal. In IntelliJ IDEA, the
[portcheck plugin](../guide/jetbrains.md) catches this in the Run window and offers
**Stop and rerun**.

## Go

```text
listen tcp :8080: bind: address already in use
```

`go run` builds a binary in a temporary folder, so the holder often has a random name
like `main` or `__debug_bin`. portcheck's `started by` (`go ← zsh`, or `dlv` for the
debugger) tells you where it came from.

## ASP.NET Core (Kestrel)

```text
System.IO.IOException: Failed to bind to address http://127.0.0.1:5000: address already in use.
```

On macOS, port 5000 is often **AirPlay Receiver**; see
[the macOS note](macos.md#port-5000-or-7000-on-a-mac). Otherwise, it's usually an earlier
`dotnet run`: `portcheck kill 5000`.

## PHP built-in server

```text
Failed to listen on localhost:8000 (reason: Address already in use)
```

`portcheck kill 8000`, or start on a free port:
`portcheck run --port 8000 --next -- php -S localhost:{port}`.

## Related

- [Node.js](node.md) · [Python](python.md) · [Docker](docker.md)
- [macOS](macos.md) · [Windows](windows.md) · [Linux](linux.md)
- [All "port already in use" fixes](index.md)
