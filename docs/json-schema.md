# JSON output

Every command that prints results (`list`, `check`, `free`, `kill`, `scan`, `wait`,
`version`) accepts `--json` (or `-o json`) and prints one JSON document. (`run`
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
| `kind` | Which command produced the document: `list`, `check`, `free`, `kill`, `scan`, `wait`, `version`. |
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

### Container

| Field | Type | Notes |
|---|---|---|
| `id` | string, optional | short (12-character) container ID (not set for WSL) |
| `name` | string, optional | container name, or the WSL distro name |
| `image` | string, optional | image reference, e.g. `nginx:alpine` |
| `runtime` | string | `docker`, `podman` or `wsl` |
| `process` | string, optional | process inside the container or distro, when known |
| `processPid` | number, optional | its PID inside the container or distro |

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
