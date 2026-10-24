---
title: "Kill the process on a port on Linux (address already in use)"
description: "Find which process is using a port on Linux and stop it: with portcheck in one command, or with ss, lsof or fuser. Works on Ubuntu, Debian, Fedora, Arch and WSL."
---

# Kill the process using a port on Linux

## The quick way

```sh
npx portcheck-cli kill 3000
```

portcheck lists what is listening on port 3000 and who started it, asks before stopping
it, and refuses system processes such as `systemd` and `init`. It needs no `lsof` or
`net-tools`, so it also works in minimal containers. To install it permanently:
`brew install roystondwork-dev/tap/portcheck`, `pipx install portcheck-tool`, or the
`.deb`, `.rpm` and `.apk` packages on the releases page (see [Install](../install.md)).

To see what's there without stopping it:

```console
$ portcheck 3000
PORT  PROTO  STATUS  PID    PROCESS  USER
3000  tcp    in-use  41852  node     alice
```

## With built-in tools

=== "ss (installed almost everywhere)"

    ```sh
    ss -ltnp 'sport = :3000'
    ```

    The `users:(("node",pid=41852,...))` part names the process. Then:

    ```sh
    kill 41852        # asks it to exit
    kill -9 41852     # forces it
    ```

=== "fuser"

    ```sh
    fuser 3000/tcp       # print the PID
    fuser -k 3000/tcp    # kill it (SIGKILL by default)
    ```

=== "lsof"

    ```sh
    lsof -nP -iTCP:3000 -sTCP:LISTEN
    ```

!!! note "No PID shown?"
    Linux only shows the owner of sockets that belong to your user. If the process
    column is empty, the port belongs to another user or root: use
    `sudo portcheck 3000` (or `sudo ss -ltnp`).

## systemd services

If the process comes back right after you stop it, a service manager is restarting it.
Such processes have no `started by` in portcheck, because the system started them, not a
terminal. Stop the service instead:

```sh
systemctl status <name>        # find which service it is
sudo systemctl stop <name>
```

## Related

- [Docker](docker.md) · [Node.js](node.md) · [Python](python.md)
- [Permissions on Linux](../permissions.md)
- [All "port already in use" fixes](index.md)
