# Quickstart

Five minutes to every feature. Each command works the same on macOS, Linux and Windows.

## 1. What's listening?

```console
$ portcheck list --listening
PROTO  LOCAL ADDRESS   REMOTE ADDRESS  STATE   PID    PROCESS        USER
tcp    127.0.0.1:3000  -               LISTEN  42379  node           alice
tcp    [::]:5432       -               LISTEN  881    postgres       postgres
udp    0.0.0.0:5353    -               NONE    300    mDNSResponder  _mdns
```

Narrow it down with `--port 3000-3999`, `--proto tcp`, `--name 'node*'` or `--pid 42379`.

## 2. Is my port free?

```console
$ portcheck 3000
PORT  PROTO  STATUS  PID    PROCESS  USER
3000  tcp    in-use  42379  node     alice
$ echo $?
1
```

`portcheck 3000` is short for `portcheck check 3000`. Exit status `0` means
every port is free and `1` means at least one is taken. You can check several at
once: `portcheck check 3000 5432 6379`.

## 3. Find a free one

```console
$ portcheck free 3000-3999 -n 3
PORT
3001
3002
3003
```

portcheck only reports ports it could actually bind, on IPv4 and IPv6 and on
loopback and all interfaces.

## 4. Free it up

```console
$ portcheck kill 3000
About to terminate:
  PID 42379  node  (port 3000)
Proceed? [y/N] y
PID    PROCESS  RESULT      DETAIL
42379  node     terminated
```

The process gets a polite SIGTERM first and is force-killed only if it's still running
after `--grace` (default 5s). Use `--dry-run` to preview, and `--yes` in scripts.

## 5. Check a remote service

```console
$ portcheck scan db.internal -p 5432,6379
HOST                    PORT  STATE   SERVICE     LATENCY  DETAIL
db.internal (10.0.0.5)  5432  open    postgresql  1.2ms
db.internal (10.0.0.5)  6379  closed  redis       800µs
Scanned 1 host(s) × 2 port(s) in 3ms: 1 open, 1 closed, 0 filtered, 0 errors.
```

!!! warning
    Only scan hosts and networks you own or are authorized to test.

## 6. Wait for a service

```sh
portcheck wait 5432 -t 60s
```

Blocks until something accepts connections on port 5432, and exits `1` if
nothing does within the timeout. See [`wait`](guide/wait.md).

## 7. Start a dev server on a free port

```sh
portcheck run --port 3000 --next -- npm run dev
```

Uses 3000 if it's free, otherwise the next free port, and passes it in `$PORT`.
See [`run`](guide/run.md).

## 8. Watch it live

```sh
portcheck
```

Running `portcheck` with no arguments in a terminal opens the [interactive view](guide/tui.md).
Press `?` for keys and `q` to quit.

## 9. Use it from scripts

```sh
portcheck list -l --json | jq '.data.sockets[] | {port: .localPort, process: .process.name}'
```

Every command supports `--json` and `-o csv|plain|table`. See [Scripting & CI](guide/scripting.md).
