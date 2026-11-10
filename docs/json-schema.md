# JSON output

Every command that prints results (`list`, `check`, `free`, `kill`, `scan`, `wait`,
`doctor`, `port`, `version`) accepts `--json` (or `-o json`) and prints one JSON document. (`run`
hands stdout to the command it starts; with `--kill`, its kill report goes to
stderr in the requested format.)

```json
{
  "schemaVersion": 1,
  "kind": "list",
  "data": { ... }
}
```

| Field | Meaning |
|---|---|
| `schemaVersion` | Version of this schema. It only increases when a field is removed or changes meaning. New optional fields can appear in any release. |
| `kind` | Which command produced the document: `list`, `check`, `free`, `kill`, `scan`, `wait`, `doctor`, `port`, `ports` (`port --list`), `version`. |
| `data` | The payload, described below. |

With `list --watch`, one document is printed per refresh.

Fields marked *optional* are omitted when unknown or empty. Arrays are always
present, and empty arrays are `[]`, never `null`.

## Shared objects

### Socket

| Field | Type | Notes |
|---|---|---|
| `proto` | string | `tcp` or `udp` |
| `localAddress` | string | IP address, e.g. `127.0.0.1`, `::`, `0.0.0.0` |
| `localPort` | number | |
| `remoteAddress` | string, optional | only for connected sockets |
| `remotePort` | number, optional | |
| `state` | string | e.g. `LISTEN`, `ESTABLISHED`, `TIME_WAIT`; `NONE` for UDP |
| `pid` | number, optional | omitted when the owner is not visible (permissions) |
| `process` | Process, optional | |
| `container` | Container, optional | the container (or WSL distro) behind this port, when it can be determined |
| `tunnel` | Tunnel, optional | where the port leads, when the process only forwards it (`ssh -L`, `ssh -D`, `kubectl`/`oc port-forward`) |

### Tunnel

Read from the process's command line; a forward configured in an SSH config file
(`LocalForward`) isn't visible there.

| Field | Type | Notes |
|---|---|---|
| `kind` | string | `ssh` (`ssh -L`), `ssh-socks` (`ssh -D`) or `port-forward` (`kubectl`/`oc port-forward`) |
| `target` | string, optional | where connections go, e.g. `db.internal:5432` or `svc/web:80`; not set for `ssh-socks` |
| `via` | string, optional | the SSH destination, e.g. `ops@bastion`, or the Kubernetes namespace |
| `description` | string | e.g. `SSH tunnel to db.internal:5432 via ops@bastion` |

### Process

| Field | Type | Notes |
|---|---|---|
| `pid` | number | |
| `name` | string | |
| `user` | string, optional | |
| `cmdline` | string, optional | |
| `exe` | string, optional | |
| `startTime` | string, optional | RFC 3339, UTC |
| `ancestors` | array, optional | processes that started this one, nearest first (up to 3): `[{ "pid": 4200, "name": "npm" }, …]`; stops before PID 1 |
| `cwd` | string, optional | the process's working directory |
| `project` | string, optional | the project it runs in: the name of the nearest directory at or above `cwd` holding `.git`, `package.json`, `go.mod`, `pyproject.toml`, `Cargo.toml` or a similar marker, else `cwd`'s name; empty for `/` and the home directory |

### Container

| Field | Type | Notes |
|---|---|---|
| `id` | string, optional | short (12-character) container ID (not set for WSL) |
| `name` | string, optional | container name, or the WSL distro name |
| `image` | string, optional | image reference, e.g. `nginx:alpine` |
| `runtime` | string | `docker`, `podman` or `wsl` |
| `process` | string, optional | process inside the container or distro, when known |
| `processPid` | number, optional | its PID inside the container or distro |
| `composeProject` | string, optional | the Docker Compose project that started the container |
| `composeService` | string, optional | its Compose service |
| `stopHint` | string | command that stops it without touching the host proxy, e.g. `docker stop web`, `docker compose -p myapp stop db` or `wsl -d Ubuntu --exec kill 1234` |

## `list`

```json
{ "sockets": [Socket...], "partial": false }
```

`partial` is `true` when some owners could not be determined without elevated
privileges.

## `check`

```json
{ "ports": [ { "port": 3000, "proto": "tcp", "status": "in-use", "holders": [Socket...] } ] }
```

`status` is one of `free`, `in-use`, `denied`. A port is `denied` when it can't be bound
for lack of privileges, for example a port below 1024 on Linux or a reserved
range on Windows. `holders` can be empty for an `in-use` port whose owner is hidden.

## `free`

```json
{ "ports": [3000, 3001] }
```

## `kill`

```json
{ "results": [ { "pid": 4242, "name": "node", "outcome": "terminated" } ] }
```

`outcome` is one of `terminated`, `killed` (needed force), `already-exited`, `dry-run`
or `failed`. A failed entry has an `error` message.

## `scan`

```json
{
  "results": [
    { "host": "db.internal", "address": "10.0.0.5", "port": 5432, "state": "open",
      "service": "postgresql", "latencyMs": 1.234 }
  ],
  "summary": { "hosts": 1, "ports": 2, "open": 1, "closed": 1, "filtered": 0, "errors": 0, "durationMs": 12 }
}
```

`state` is one of `open`, `closed`, `filtered` (timed out) or `error`. An `error` result has
an `error` message. `service` is the port's well-known name, not something
detected on the wire. With `--open`, `results` contains only open ports, but
`summary` still counts every probe.

## `wait`

```json
{
  "targets": [
    { "target": "db:5432", "host": "db", "port": 5432, "condition": "open",
      "ready": true, "waitedMs": 1250, "attempts": 6 }
  ]
}
```

`condition` is `open` or `free`. `target` is the argument as given; `ready: false` means it timed out.

## `doctor`

```json
{
  "config": "/work/shop/.portcheck.yaml",
  "ok": false,
  "checks": [
    { "name": "web", "port": 3000, "proto": "tcp", "expect": "free", "ok": false, "status": "in-use",
      "detail": "in use by node (pid 4242), started by npm (4200)", "fix": "portcheck kill 3000" },
    { "name": "postgres", "port": 5432, "proto": "tcp", "expect": "open", "host": "localhost", "ok": true, "status": "ok" }
  ]
}
```

`status` is `ok`, `in-use`, `denied` (expect free), `closed` (expect open) or `error`.
`detail` and `fix` are human-readable and may change between releases; match on `ok`
and `status` in scripts.

## `explain`

`data` describes the ports a command line probably listens on and which of them are taken:

| Field | Type | Meaning |
|---|---|---|
| `command` | string | the command line given |
| `exitCode` | integer | the `--exit` status (default 1) |
| `ports` | array | candidate ports, in the order found: `[{ "port": 3000, "source": "explicit" }]`. `source` is `explicit` (on the command line or in a `…PORT=` variable) or `default` (a known dev server's default port). Empty for exit statuses 0, 126, 127, 130, 148 and for portcheck itself |
| `inUse` | array | the candidate ports something holds, as in [`check`](#check) (at most 3) |

With the holder's [socket](#socket) shortened:

```json
{
  "schemaVersion": 1,
  "kind": "explain",
  "data": {
    "command": "vite --port 3000",
    "exitCode": 1,
    "ports": [{ "port": 3000, "source": "explicit" }],
    "inUse": [{ "port": 3000, "proto": "tcp", "status": "in-use", "holders": [ { "pid": 41852, "process": { "name": "node" } } ] }]
  }
}
```

## `browser`

`portcheck browser status`, `install` and `uninstall` print the browsers they looked at:

```json
{
  "host": "io.github.roystondwork_dev.portcheck",
  "browsers": [
    { "browser": "chrome", "name": "Google Chrome", "detected": true, "registered": true,
      "manifest": "/home/alice/.config/google-chrome/NativeMessagingHosts/io.github.roystondwork_dev.portcheck.json",
      "path": "/usr/local/bin/portcheck", "extensions": ["fmocabgaaihiflefcdafnhoihcibgnhl"] },
    { "browser": "edge", "name": "Microsoft Edge", "detected": false, "registered": false,
      "manifest": "/home/alice/.config/microsoft-edge/NativeMessagingHosts/io.github.roystondwork_dev.portcheck.json" }
  ]
}
```

`detected` means the browser seems to be installed for this user; `registered` means
portcheck is its native messaging host, starting `path` for the extension IDs in
`extensions`.

## `port`

`portcheck port` (and `--set`, `--forget`):

```json
{ "port": 3456, "project": "shop", "path": "/home/alice/code/shop", "assigned": false }
```

`assigned` is `true` when this call gave the project its port (the first time, or with
`--set`). `--forget` adds `"forgotten": true`.

## `ports`

`portcheck port --list`:

```json
{ "projects": [ { "port": 3456, "project": "shop", "path": "/home/alice/code/shop", "lastUsed": "2026-11-09T12:00:00Z", "exists": true } ] }
```

`exists` is `false` when the project's folder is gone.

## `net`

```json
{
  "interfaces": [
    { "name": "en0", "mac": "00:00:5e:00:53:0a", "mtu": 1500, "kind": "physical",
      "addresses": [ { "address": "192.168.1.10/24", "class": "private", "version": "IPv4" } ] }
  ],
  "primary": { "ipv4": "192.168.1.10", "ipv6": "2001:db8::10", "interface": "en0" },
  "gateways": ["192.168.1.1"],
  "dns": ["192.168.1.1"],
  "public": ["203.0.113.7"],
  "warnings": ["DNS servers: not supported on this operating system"]
}
```

| Field | Notes |
|---|---|
| `interfaces[].kind` | `loopback`, `vpn`, `virtual` or `physical` (a guess from the name) |
| `addresses[].class` | `loopback`, `link-local`, `private`, `shared` (100.64.0.0/10), `multicast`, `unspecified` or `public` |
| `primary` | the addresses used to reach the internet; omitted fields mean no such route |
| `public` | only with `--public` (from ipify) |
| `warnings` | facts that couldn't be read; also printed on stderr |

## `devices`

From `portcheck net devices`.

```json
{
  "devices": [
    { "ip": "192.168.1.1", "mac": "00:00:5e:00:53:01", "name": "router.lan", "interface": "en0", "gateway": true },
    { "ip": "192.168.1.10", "mac": "00:00:5e:00:53:0a", "interface": "en0", "self": true },
    { "ip": "192.168.1.20", "mac": "b8:27:eb:00:00:20", "vendor": "Raspberry Pi", "interface": "en0" }
  ],
  "probed": "192.168.1.0/24",
  "warnings": []
}
```

`vendor` is a hint from the MAC address, or `private (randomized) MAC`. `probed` is the
network probed with `--probe`. A device found by probing but missing from the neighbour
cache has only `ip`.

## `lookup`

From `portcheck net lookup`. One result per query, in the order given.

```json
{
  "results": [
    { "query": "example.com", "kind": "domain", "records": [
        { "type": "A", "value": "192.0.2.80", "note": "public" },
        { "type": "MX", "value": "mail.example.com", "note": "preference 10" } ] },
    { "query": "192.168.1.20", "kind": "ip", "records": [
        { "type": "CLASS", "value": "private", "note": "IPv4" },
        { "type": "LOCAL", "value": "on a local network", "note": "en0" },
        { "type": "MAC", "value": "b8:27:eb:00:00:20", "note": "Raspberry Pi" } ] },
    { "query": "nope.invalid", "kind": "domain", "records": [], "error": "nope.invalid doesn't resolve: …" }
  ]
}
```

Record types: `A`, `AAAA`, `CNAME`, `MX`, `NS`, `TXT` and `PTR`, plus `CLASS`, `LOCAL`
(`this machine` or `on a local network`, with the interface in `note`) and `MAC` for
addresses.

## `version`

```json
{ "version": "v0.1.0", "commit": "…", "date": "…", "modified": false, "goVersion": "go1.27.3", "platform": "darwin/arm64" }
```

## Examples with jq

```sh
# PIDs listening on 3000-3999
portcheck list -l --port 3000-3999 --json | jq '.data.sockets[].pid' | sort -u

# Fail a CI step if any port is taken, printing who holds it
portcheck check 5432 6379 --json | jq -e '.data.ports | all(.status == "free")'
```
