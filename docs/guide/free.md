# Find free ports: `free`

```console
$ portcheck free
PORT
1024

$ portcheck free 3000-3999 -n 3
PORT
3000
3001
3003
```

`free [range]` returns the lowest free ports in a [port spec](../port-specs.md)
(default `1024-65535`, which skips privileged ports). A port counts as free with the
same rules as [`check`](check.md#statuses).

| Flag | Meaning |
|---|---|
| `-n`, `--count N` | return N ports (default 1) |
| `--random` | return one random free port from the range instead of the lowest |
| `--proto udp` | look for free UDP ports |

## In scripts

`-o plain` prints a `PORT` header followed by one port per line:

```sh
PORT=$(portcheck free 8000-8999 -o plain | tail -n 1)
python3 -m http.server "$PORT"
```

or with JSON:

```sh
PORT=$(portcheck free --random 20000-30000 --json | jq '.data.ports[0]')
```

The exit status is `1` if fewer than N free ports exist in the range. The ports it did find are still printed.

!!! tip "Random ports avoid collisions"
    When several jobs start at the same time (for example, parallel CI tests),
    `--random` makes it much less likely that two of them pick the same port.
