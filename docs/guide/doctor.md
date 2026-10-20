# Project ports: `.portcheck.yaml` and `doctor`

List the ports your project needs in a `.portcheck.yaml` at the repository root, and
`portcheck doctor` tells anyone on the team what's wrong and how to fix it:

```console
$ portcheck doctor
NAME      PORT  EXPECT  STATUS  DETAIL                                                    FIX
web       3000  free    in-use  in use by node (pid 4242), started by npm (4200)          portcheck kill 3000
api       8080  free    ok
postgres  5432  open    closed  nothing accepts connections on localhost:5432             start postgres (docker compose up -d postgres)
redis     6379  open    ok
2 of 4 checks need attention (/work/shop/.portcheck.yaml).
```

## The file

```yaml
# yaml-language-server: $schema=https://roystondwork-dev.github.io/portcheck/schema/portcheck.schema.json
version: 1
ports:
  - name: web
    port: 3000
    description: Next.js dev server
  - name: api
    port: 8080
  - name: postgres
    port: 5432
    expect: open
    description: docker compose up -d postgres
  - name: redis
    port: 6379
    expect: open
```

| Key | Required | Meaning |
|---|---|---|
| `version` | yes | always `1` |
| `ports[].name` | yes | a short name: letters, digits, `.`, `_`, `-`; not only digits; unique |
| `ports[].port` | yes | 1–65535 |
| `ports[].expect` | no | **`free`** (default): nothing may listen, so the project can use it. **`open`**: a service must accept connections |
| `ports[].proto` | no | `tcp` (default) or `udp` (only with `expect: free`) |
| `ports[].host` | no | where an `open` port is checked (default `localhost`) |
| `ports[].description` | no | shown in the fix for `open` ports, e.g. how to start the service |

portcheck looks for `.portcheck.yaml` (or `.portcheck.yml`) in the current directory
and then each parent directory, like `.editorconfig`, so `doctor` works from anywhere
in the repository. `--config path` uses a specific file.

The file is checked strictly: unknown keys such as a misspelled `exepct:` are reported
with their line number instead of being ignored.

!!! tip "Editor support"
    The first line above points editors (VS Code with the YAML extension, JetBrains IDEs, …)
    at the [JSON Schema](../schema/portcheck.schema.json), which gives you completion,
    descriptions and validation as you type.

## What `doctor` checks

| `expect` | Passes when | Otherwise | Suggested fix |
|---|---|---|---|
| `free` | nothing listens and the port can be bound (same rules as [`check`](check.md#statuses)) | `in-use`, with the holder and who started it | `portcheck kill <port>`; `docker stop <name>` or the `wsl` command for [containers & WSL](containers.md); `sudo portcheck kill <port>` when the owner is hidden |
| `free` | | `denied`: your user can't bind it | use a port ≥ 1024 or elevated privileges |
| `open` | a TCP connection to `host:port` succeeds within 2 seconds | `closed` | `start <name>` plus the entry's `description` |

All checks run in parallel. The result is printed in any [output format](scripting.md#output-formats);
`--json` gives `{ "config": …, "ok": …, "checks": [ … ] }` (see [JSON output](../json-schema.md#doctor)).

## Exit status

| Code | Meaning |
|---|---|
| `0` | every check passed |
| `1` | at least one check needs attention |
| `2` | no `.portcheck.yaml` found, or it is invalid |

That makes `doctor` a natural first step in a setup script or a pre-commit/pre-push hook:

```sh
portcheck doctor || exit 1
```

## Using names with `run`

Port names work with [`run`](run.md):

```sh
portcheck run --port web --next -- npm run dev    # uses 3000 from .portcheck.yaml
```
