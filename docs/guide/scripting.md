# Scripting & CI

portcheck is designed to be driven by scripts:

- **Results go to stdout; messages go to stderr.** Hints, warnings, progress and
  summaries never break a pipe.
- **Exit codes are stable** and mean the same thing on every command. See [Exit codes](../exit-codes.md).
- **`--json` output is versioned.** See [JSON output](../json-schema.md).
- **No prompts without a terminal.** `kill` requires `--yes` when stdin isn't a TTY.
- **No colors when piped.** Colors are also off when `NO_COLOR` is set or with `--no-color`.

## Output formats

| Flag | Use for |
|---|---|
| `-o table` | humans (default on a terminal) |
| `-o plain` | `awk`, `cut`, `grep` (default when piped): aligned columns, one header line |
| `-o csv` | spreadsheets, `mlr`, `csvkit` |
| `--json` / `-o json` | `jq` and programs |

## Recipes

Fail fast if required ports are taken:

```sh
portcheck check 5432 6379 8080 || { echo "free these ports first" >&2; exit 1; }
```

Free a port before starting a dev server:

```sh
portcheck kill 3000 --yes >/dev/null 2>&1 || true
npm run dev
```

Pick a free port for a test server:

```sh
PORT=$(portcheck free --random 20000-40000 --json | jq '.data.ports[0]')
```

Wait for a service to come up:

```sh
timeout 60 sh -c 'until portcheck scan localhost -p 5432 --open >/dev/null 2>&1; do sleep 1; done'
```

List listening processes as JSON objects:

```sh
portcheck list -l --json | jq -c '.data.sockets[] | {port: .localPort, pid, name: .process.name}'
```

## GitHub Actions

```yaml
- name: Install portcheck
  run: go install github.com/roystondwork-dev/portcheck/cmd/portcheck@latest

- name: Ensure test ports are free
  run: portcheck check 5432 6379

- name: Wait for Postgres
  run: timeout 60 sh -c 'until portcheck scan localhost -p 5432 --open; do sleep 1; done'
```

## Debugging

`-v` logs diagnostics, such as how long socket listing took, to stderr. Add
`--log-format json` for structured logs.
