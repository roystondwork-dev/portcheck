# Run on a free port: `run`

`portcheck run` starts a command on a usable port in one step. The port is passed
in `$PORT`, and every `{port}` in the command's arguments is replaced.

```console
$ portcheck run --port 3000 --next -- npm run dev
portcheck: port 3000 is in use by node (pid 42379); using 3001
portcheck: starting npm run dev with PORT=3001
…your dev server's output…
```

## Choosing the port

| You pass | What happens if the port is taken |
|---|---|
| `--port 3000` | portcheck explains who holds it and exits `1`; the command isn't started |
| `--port 3000 --next` | uses the next free port above 3000 |
| `--port 3000 --kill` | stops the holder (asks first; `--yes` skips the question), waits for the port to be released, then uses 3000 |
| *(no `--port`)* | uses the first free port in `--range` (default `3000-9999`) |

"Free" uses the same rules as [`check`](check.md#statuses). `--kill` uses the same
confirmation and [safety guards](kill.md#safety) as `portcheck kill`: it refuses
system processes, and for a port published by a container it tells you to
`docker stop` the container instead (see [Containers](containers.md)).

## Passing the port to your command

- **`$PORT`** is set for the command, replacing any existing value. Many frameworks
  read it directly. Use `--env NAME` for tools that expect a different variable,
  e.g. `--env VITE_PORT`.
- **`{port}`** in arguments is replaced, for tools that take the port as an argument:

```sh
portcheck run -- python3 -m http.server {port}
portcheck run --port 5173 --next -- npx vite --port {port}
portcheck run --env SERVER_PORT -- ./gradlew bootRun
```

Use `--` before the command when it has flags of its own. portcheck stops reading
its own flags at the command name anyway, so `portcheck run --port 3000 npm start --verbose`
also works.

## Readiness

`--wait` prints a line once the command accepts connections, which is handy when a
dev server takes a while to compile:

```console
$ portcheck run --port 8080 --wait -- ./gradlew bootRun
portcheck: starting ./gradlew bootRun with PORT=8080
…
portcheck: ready on localhost:8080 (14.2s)
```

If nothing accepts connections within `--wait-timeout` (default `60s`), portcheck
says so; the command keeps running either way.

## Running the command

The command runs in the foreground with your terminal: same input, output and
working directory. **Ctrl+C reaches it directly**, and portcheck forwards
`SIGTERM` and `SIGHUP` to it on macOS and Linux, so it can shut down cleanly.

portcheck's own messages go to **stderr** (silence them with `-q`), so the
command's stdout is untouched.

## Exit status

`portcheck run` exits with **the command's exit status**, like `env` or `time`. If
the command is killed by a signal, the status is `128 + signal` (for example `143`
for `SIGTERM`), as in a shell.

When portcheck can't start the command:

| Code | Meaning |
|---|---|
| `1` | the requested port is taken (without `--next`/`--kill`), no free port in the range, or `--kill` was refused |
| `2` | invalid flags, e.g. `--next` without `--port` |
| `4` | `--kill` on a port your user can't bind |
| `126` | the command exists but can't be executed |
| `127` | the command wasn't found |

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-p`, `--port` | — | port to use |
| `--next` | off | if `--port` is taken, use the next free port above it |
| `--kill` | off | if `--port` is taken, stop the process holding it |
| `-y`, `--yes` | off | with `--kill`: don't ask |
| `-f`, `--force`, `--grace` | off, `5s` | with `--kill`: as in [`kill`](kill.md#flags) |
| `--range` | `3000-9999` | where to look when `--port` isn't given |
| `--env` | `PORT` | environment variable that receives the port |
| `--wait`, `--wait-timeout` | off, `60s` | report when the command accepts connections |
| `-q`, `--quiet` | off | no portcheck messages |
