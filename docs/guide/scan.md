# Scan remote hosts: `scan`

!!! warning "Authorized use only"
    Only scan hosts and networks you own or are explicitly authorized to test.
    Scanning other people's systems may be illegal where you live and is often
    against your provider's terms. portcheck prints a reminder when a target is a
    public (internet-routable) address.

```console
$ portcheck scan db.internal 10.0.0.6 -p 22,5432
HOST                    PORT  STATE     SERVICE     LATENCY  DETAIL
db.internal (10.0.0.5)  22    closed    ssh         700µs
db.internal (10.0.0.5)  5432  open      postgresql  1.2ms
10.0.0.6                22    filtered  ssh         1.0002s
10.0.0.6                5432  filtered  postgresql  1.0001s
Scanned 2 host(s) × 2 port(s) in 1.004s: 1 open, 1 closed, 2 filtered, 0 errors.
```

## How it works

portcheck opens ordinary TCP connections ("connect scan"), the same thing any
client does. It needs **no root or Administrator rights** and has **no stealth or
evasion features**. See [ADR-0002](../adr/0002-tcp-connect-only-scanning.md) for
why.

| State | Meaning |
|---|---|
| `open` | the connection was established, so something is listening |
| `closed` | the host answered with a refusal, so nothing is listening |
| `filtered` | no answer within `--timeout`, often because of a firewall |
| `error` | another failure, e.g. *no route to host* (see `DETAIL`) |

`SERVICE` is the port's well-known name (e.g. `postgresql` for 5432). It is a label only;
portcheck does not identify what is actually running.

## Targets

Hosts can be given as hostnames, IPv4/IPv6 addresses (`[::1]` or `::1`), or CIDR
prefixes (`10.0.0.0/28`). Duplicate addresses are scanned once. Hostnames resolve to
their first IPv4 address, or their first IPv6 address if they have none.

To avoid accidents, a scan is limited to **1024 hosts**; raise it with `--max-hosts`.

## Ports

`-p` takes a [port spec](../port-specs.md); the default is the `common` preset (about 45
ports developers commonly use).

```sh
portcheck scan 10.0.0.0/28 -p 22,80,443 --open
portcheck scan example.internal -p well-known
```

## Speed and politeness

| Flag | Default | Meaning |
|---|---|---|
| `--concurrency` | `100` | simultaneous connection attempts |
| `--rate` | `500` | maximum new connections per second (`0` = unlimited) |
| `--timeout` | `1s` | per-connection timeout; slower networks may need more |
| `--retries` | `0` | extra attempts for `filtered` ports (useful on lossy links) |

In a terminal, a live counter shows progress on stderr. Press Ctrl-C to stop;
results gathered so far are still printed.

## Output and exit status

`--open` hides closed, filtered and error results; the summary still counts them.
`--json` output is documented in [JSON output](../json-schema.md#scan).

| Code | Meaning |
|---|---|
| `0` | at least one port is open |
| `1` | no open ports |
| `2` | invalid target, port spec or flags, or too many hosts |
| `3` | a hostname did not resolve |
| `130` | interrupted (partial results printed) |

To block until a service accepts connections, use [`portcheck wait`](wait.md) instead of a loop.
