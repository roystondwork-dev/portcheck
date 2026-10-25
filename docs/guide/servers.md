# Forgotten dev servers: `servers`

```console
$ portcheck servers
PID    PROCESS  PROJECT   PORTS       UPTIME
41852  node     my-shop   5173,24678  2d 6h
51230  python3  api       8000        3h 12m
60411  hugo     docs      1313        25m
```

`servers` lists the dev servers, databases and tools **you** started that are still
listening on a TCP port, oldest first, with the project each one runs in. The ones at
the top, running for days, are usually the ones you forgot about: the reason your next
`npm run dev` says "port already in use", or why your laptop's fan is on.

## What counts as a server

A process is listed when:

- it listens on a TCP port;
- it belongs to you (other users' processes and containers are left out);
- it runs inside a **project folder**.

The project is the nearest folder at or above the process's working directory that
holds `.git`, `package.json`, `go.mod`, `pyproject.toml`, `Cargo.toml`, `Gemfile`,
`pom.xml`, `build.gradle`, `composer.json`, `deno.json` or `mix.exs`. Without one, it's
the working directory's own name. Apps that run from `/` or your home folder, such as
AirPlay Receiver, Spotify or your editor's helpers, aren't in a project, so they're left
out. `--all` includes them:

```console
$ portcheck servers --all
PID    PROCESS        PROJECT  PORTS       UPTIME
676    ControlCenter  -        5000,7000   1d 20h
41852  node           my-shop  5173,24678  2d 6h
…
```

## Only the old ones

```sh
portcheck servers --older-than 1d     # running for a day or more
portcheck servers --older-than 90m    # any Go duration: 30m, 12h, 1h30m
```

## Stop them

```console
$ portcheck servers --older-than 1d --stop
About to terminate:
  PID 41852  node  (port 5173, 24678), started by npm (41850) ← zsh (900), in my-shop
Proceed? [y/N] y
PID    PROCESS  RESULT      DETAIL
41852  node     terminated
```

`--stop` asks first and stops each server the same way, and with the same safety
guards, as [`kill`](kill.md): a graceful request, then a forced stop after `--grace`
(5 s). `--dry-run` shows what would be stopped; `--yes` skips the question.

`kill` shows the project too, so you can tell your two `node` processes apart:

```console
$ portcheck kill 3000
About to terminate:
  PID 42379  node  (port 3000), started by npm (4200) ← zsh (900), in my-shop
Proceed? [y/N]
```

## In scripts

`servers` exits with `1` when nothing matches, so you can check for leftovers:

```sh
if portcheck servers --older-than 1d >/dev/null; then
  echo "You have dev servers running for over a day: portcheck servers"
fi
```

With `--json`, each server has the [process fields](../json-schema.md) (including
`cwd` and `project`), its `ports`, and `uptimeSeconds`.

## Where the information comes from

The working directory is read from the operating system (`proc_pidinfo` on macOS,
`/proc` on Linux, the process environment block on Windows). It's only readable for
your own processes, which is all `servers` lists. Start times come from the same place
as in [`list`](list.md).
