# Check a port: `check`

`portcheck check` tells you whether local ports are free and, if not, which process holds them.

```console
$ portcheck check 3000 3001 80
PORT  PROTO  STATUS  PID    PROCESS  USER
3000  tcp    in-use  42379  node     alice
3001  tcp    free    -      -        -
80    tcp    denied  -      -        -
```

`portcheck <port>` is a shortcut for `portcheck check <port>`. Arguments are
[port specs](../port-specs.md), so `portcheck check 8000-8010` works too (up to 4096 ports).

## Statuses

| Status | Meaning |
|---|---|
| `free` | No listener, and portcheck could bind the port on `127.0.0.1`, `0.0.0.0`, `::1` and `::` (addresses your system doesn't support, e.g. IPv6, are skipped) |
| `in-use` | A process listens on the port, or binding failed because the address is in use |
| `denied` | Binding failed for lack of privileges: ports below 1024 without root on Linux (and on specific addresses on macOS), or a [Windows excluded port range](../troubleshooting.md#windows-says-a-port-is-denied-but-nothing-uses-it) |

!!! info "Why both checks?"
    Some systems let you bind `0.0.0.0:3000` even though another program already
    listens on `127.0.0.1:3000`. So portcheck looks for listeners **and** tries to bind
    every address, and reports `free` only when both agree.

An `in-use` port with no PID means the owner is hidden from your user; portcheck
prints a hint on how to see it.

## UDP

```sh
portcheck check 53 --proto udp
```

## Exit status

| Code | Meaning |
|---|---|
| `0` | every port is free |
| `1` | at least one port is `in-use` or `denied` |
| `2` | invalid arguments |

```sh
portcheck check 5432 >/dev/null || { echo "Postgres port is taken"; exit 1; }
```

!!! warning "Races"
    A port reported free can be taken a moment later. When your own program needs
    a port, bind it directly, or bind port `0` and let the OS choose.
