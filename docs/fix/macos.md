---
title: "Kill the process on a port on macOS (port 3000 already in use)"
description: "Find which process is using a port on a Mac and stop it: with portcheck in one command, or with lsof and kill. Includes AirPlay Receiver on port 5000."
---

# Kill the process using a port on macOS

## The quick way

```sh
npx portcheck-cli kill 3000
```

portcheck lists what is listening on port 3000 and who started it, asks before stopping
it, and never touches macOS system processes such as `launchd` or `WindowServer`. To
install it permanently: `brew install roystondwork-dev/tap/portcheck`.

To see what's there without stopping it:

```console
$ portcheck 3000
PORT  PROTO  STATUS  PID    PROCESS  USER
3000  tcp    in-use  41852  node     alice
```

Or browse every port your Mac is listening on, and stop one with ++x++:

```sh
portcheck
```

## With built-in tools (lsof and kill)

macOS ships `lsof`, which can show the process on a port:

```console
$ lsof -nP -iTCP:3000 -sTCP:LISTEN
COMMAND   PID  USER   FD   TYPE  DEVICE SIZE/OFF NODE NAME
node    41852 alice   23u  IPv6  0x...       0t0  TCP *:3000 (LISTEN)
```

Then stop it by PID:

```sh
kill 41852        # asks it to exit
kill -9 41852     # forces it, if it ignores the first
```

The common one-liner `lsof -ti:3000 | xargs kill -9` does both at once, but it also
matches programs that are merely *connected* to port 3000 (like your browser), and
`-9` gives the server no chance to save anything. Check what you're stopping first.

!!! note "Processes owned by other users"
    Without `sudo`, macOS hides processes that belong to other users and root.
    If nothing shows up but the port is still busy, run `sudo portcheck 3000`.

## Port 5000 or 7000 on a Mac

On macOS 12 and later, **AirPlay Receiver** (the `ControlCenter` process) listens on
ports 5000 and 7000. Don't kill it, as it restarts. Turn it off instead: **System Settings →
General → AirDrop & Handoff → AirPlay Receiver**, or run your app on another port. See
[Python, Django, Flask](python.md#port-5000-on-macos-airplay-receiver).

## Related

- [Node.js, Next.js, Vite, React](node.md) · [Python](python.md) · [Docker](docker.md)
- [Permissions on macOS](../permissions.md)
- [All "port already in use" fixes](index.md)
