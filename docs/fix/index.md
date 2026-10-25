---
title: "Port already in use: find and stop the process"
description: "Fix “address already in use” (EADDRINUSE) on macOS, Windows and Linux: see which process holds the port, who started it, and stop it safely with one command."
---

# Port already in use? Find and stop the process

Something is already listening on the port your app wants. To see what it is and stop it,
run this. Nothing needs to be installed first:

```sh
npx portcheck-cli kill 3000
```

```console
About to terminate:
  PID 42379  node  (port 3000), started by npm (4200) ← zsh (900)
Proceed? [y/N] y
PID    PROCESS  RESULT      DETAIL
42379  node     terminated
```

portcheck shows you **what** it is about to stop and **who started it**, then waits for
your yes. It asks the process to exit cleanly before forcing it, and it refuses to stop
system processes, so a mistyped port can't take down your Mac or PC.

To only look, without stopping anything:

```sh
npx portcheck-cli 3000
```

!!! tip "Install it to skip npx"
    `brew install roystondwork-dev/tap/portcheck` (macOS, Linux),
    `npm install -g portcheck-cli` or `pipx install portcheck-tool` (any OS), or
    Scoop on Windows. Then it's just `portcheck kill 3000`.
    See [Install](../install.md) for every option.

## Find your error

| You see | Page |
|---|---|
| `Error: listen EADDRINUSE: address already in use :::3000` | [Node.js, Next.js, Vite, React](node.md) |
| `Port 5173 is in use, trying another one...` | [Node.js, Next.js, Vite, React](node.md) |
| `OSError: [Errno 48] Address already in use` (or `[Errno 98]`) | [Python, Django, Flask](python.md) |
| `Error: That port is already in use.` | [Python, Django, Flask](python.md) |
| `Port 5000 is in use by another program` | [Python, Django, Flask](python.md#port-5000-on-macos-airplay-receiver) |
| `Bind for 0.0.0.0:8080 failed: port is already allocated` | [Docker](docker.md) |
| `Address already in use - bind(2)` / `A server is already running` | [Rails, Spring Boot, Go and others](others.md) |
| `Web server failed to start. Port 8080 was already in use.` | [Rails, Spring Boot, Go and others](others.md) |
| `listen tcp :8080: bind: address already in use` | [Rails, Spring Boot, Go and others](others.md) |
| `Only one usage of each socket address ... is normally permitted` | [Windows](windows.md) |
| `Is another postmaster already running on port 5432?` | [PostgreSQL, MySQL, Redis, MongoDB](databases.md) |
| `Do you already have another mysqld server running on port: 3306 ?` | [PostgreSQL, MySQL, Redis, MongoDB](databases.md) |
| `Could not create server TCP listening socket *:6379` | [PostgreSQL, MySQL, Redis, MongoDB](databases.md) |
| `nginx: [emerg] bind() to 0.0.0.0:80 failed (98: Address already in use)` | [nginx, Apache, Caddy, IIS](web-servers.md) |
| `AH00072: make_sock: could not bind to address [::]:80` | [nginx, Apache, Caddy, IIS](web-servers.md) |
| Port 80 held by `System` (PID 4) on Windows | [nginx, Apache, Caddy, IIS](web-servers.md#windows-port-80-held-by-system-pid-4) |
| `error listen EADDRINUSE: address already in use :::8081` | [React Native, Expo, adb](mobile.md) |
| `cannot bind to 127.0.0.1:5037: Address already in use` | [React Native, Expo, adb](mobile.md#android-adb-on-port-5037) |

Or go by operating system: [macOS](macos.md) · [Windows](windows.md) · [Linux](linux.md).

## Why it happens

Only one program can listen on a port at a time. Usually the one holding it is:

- **your own dev server from earlier**, still running in another terminal, a closed
  terminal tab, or the background;
- a server that **crashed or was suspended** (++ctrl+z++) without releasing the port;
- **another app** that uses the same default port, such as AirPlay Receiver on port 5000
  on macOS, or a database or container you forgot about.

portcheck's `started by` column usually tells you which one: `npm ← zsh` is a terminal
you started, `Code Helper` is your editor, `docker:web` is a container.

## Stop it happening again

Instead of freeing the port each time, start your app on a free port:

```sh
portcheck run --port 3000 --next -- npm run dev
```

This uses port 3000 if it's free and the next free port if not, and passes it to your
app in `$PORT`. See [Run on a free port](../guide/run.md). In VS Code, JetBrains IDEs,
Neovim and the [browser](../guide/browser.md), portcheck names the holder as soon as your
app hits the error.

## Compared with kill-port

`npx kill-port 3000` is the other common answer. It force-kills whatever is listening,
straight away. See [portcheck vs kill-port](kill-port.md) for the differences.
