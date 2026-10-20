# Wait for ports: `wait`

`portcheck wait` blocks until every target accepts TCP connections, or, with
`--free`, until every local port is free. It's made for scripts and CI, where a
service has to be up before the next step runs.

```console
$ portcheck wait 5432 -t 60s
Waiting for localhost:5432 to accept connections (timeout 1m0s)…
TARGET  CONDITION  STATUS  WAITED  ATTEMPTS
5432    open       ready   3.251s  14
```

## Targets

| Form | Example | Meaning |
|---|---|---|
| port | `5432` | port on `--host` (default `localhost`) |
| host:port | `db:5432`, `10.0.0.5:6379` | a remote service |
| IPv6 | `[::1]:8080` | brackets required |

Several targets are awaited **in parallel**; the command succeeds only when all of them are ready.

```sh
portcheck wait db:5432 redis:6379 queue:5672 -t 2m
```

`localhost` is tried on every address it resolves to (IPv4 and IPv6), so it works
whichever one the service listens on.

## Waiting for a port to be free

```sh
portcheck kill 3000 --yes
portcheck wait 3000 --free -t 10s && npm run dev
```

`--free` uses the same rules as [`check`](check.md#statuses): no visible listener and
bindable on every local address. It only works with local ports.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-t`, `--timeout` | `30s` | give up after this long; `0` waits forever |
| `--interval` | `250ms` | time between attempts (minimum `10ms`) |
| `--host` | `localhost` | host for targets given as a bare port |
| `--free` | off | wait until ports are free instead of open |

Each connection attempt is limited to 2 seconds, so a firewall that silently drops
packets is retried instead of using up the whole timeout.

## Exit status

| Code | Meaning |
|---|---|
| `0` | every target is ready |
| `1` | timed out; the table shows which targets weren't ready |
| `2` | invalid target or flags |
| `130` | interrupted with Ctrl-C (results so far are printed) |

The "Waiting for…" line goes to stderr and the result table to stdout, so
`portcheck wait 5432 >/dev/null` stays quiet in CI except on the progress line.

!!! tip "Open isn't the same as healthy"
    `wait` checks that a port **accepts TCP connections**. Some servers accept
    connections a little before they can serve requests. For HTTP services, follow
    up with a real health check (`curl --retry`), or use your database's own
    readiness tool (`pg_isready`).
