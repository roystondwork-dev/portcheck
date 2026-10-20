# Inspect ports: `list`

`portcheck list` (alias `ls`) shows local TCP and UDP sockets together with the process that owns each one.

```console
$ portcheck list --listening
PROTO  LOCAL ADDRESS   REMOTE ADDRESS  STATE   PID    PROCESS        USER
tcp    0.0.0.0:22      -               LISTEN  -      ?              ?
tcp    127.0.0.1:3000  -               LISTEN  42379  node           alice
udp    0.0.0.0:5353    -               NONE    300    mDNSResponder  _mdns
hint: Some process details are hidden. Re-run with sudo to see owners of all sockets.
```

A `-` PID and `?` process mean the owner is hidden from your user (this example is
from Linux; on macOS such sockets are not listed at all). The hint (on stderr)
says how to see it. See [Permissions by OS](../permissions.md).

## Filters

Filters combine with AND.

| Flag | Example | Matches |
|---|---|---|
| `-l`, `--listening` | `-l` | TCP sockets in `LISTEN` and unconnected UDP sockets |
| `--port` | `--port 3000-3999,8080` | local port ([port spec](../port-specs.md)) |
| `--proto` | `--proto tcp` | `tcp` and/or `udp` |
| `--pid` | `--pid 4242 --pid 4243` | owning process IDs |
| `--name` | `--name 'node*'` | process or container name, case-insensitive [glob](https://pkg.go.dev/path#Match) |
| `--state` | `--state ESTABLISHED,TIME_WAIT` | socket state (case-insensitive) |

```sh
portcheck list --name 'java*' --state established     # who is java talking to?
portcheck list --port well-known -l                   # privileged listeners
```

## Who started it? (`--wide`)

`-w`/`--wide` adds a **PARENT** column (the process that started it) and the
**COMMAND** line (shortened to 60 characters; the full value is in `--json`):

```console
$ portcheck list -l --port 3000 --wide
PROTO  LOCAL ADDRESS   REMOTE ADDRESS  STATE   PID    PROCESS  USER   PARENT      COMMAND
tcp    127.0.0.1:3000  -               LISTEN  42379  node     alice  npm (4200)  node ./node_modules/.bin/next dev
```

JSON output includes up to three ancestors, nearest first, in `process.ancestors`. The
chain stops before PID 1 (`init`, `launchd`): a process whose parent has exited is
re-parented there and shows no parent.

## Containers

Ports published by Docker or Podman containers show the container (e.g.
`docker:web`) instead of the proxy process holding them. See
[Containers](containers.md).

## Watching

`--watch` refreshes the list at an interval until you press Ctrl-C:

```sh
portcheck list -l --watch 2s
```

The screen is cleared between refreshes when output is a terminal. For an
interactive view with sorting, filtering and kill, use [`portcheck tui`](tui.md).

## Output

Sockets are sorted by local port, protocol, local address, remote address and PID.
IPv4 and IPv6 listeners on the same port are separate sockets, so both appear.

`--json` output is documented in [JSON output](../json-schema.md#list); `partial: true`
means some owners were hidden.
