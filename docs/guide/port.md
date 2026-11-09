# Stable project ports: `port`

```console
$ cd ~/code/shop
$ portcheck port
portcheck: shop's port is now 3456 (/Users/alice/code/shop)
3456
$ portcheck port
3456
```

Every project gets **its own port**, assigned the first time and remembered on your
machine. Your shop always runs on 3456, your API on 3812, and two projects never fight
over 3000 again.

Use it wherever you'd type a port:

```sh
PORT=$(portcheck port) npm run dev
portcheck run --port auto -- npm run dev          # the same, and explains a busy port
portcheck run --port auto -- python3 manage.py runserver {port}
```

The plain output is just the number, so `$(portcheck port)` works in any shell script.

## How a port is chosen

- **The project** is the nearest folder at or above the current directory that holds
  `.git`, `package.json`, `go.mod`, `pyproject.toml`, `Cargo.toml` or a similar file, so
  `portcheck port` gives the same answer anywhere inside it. Without one, it's the current
  folder. Your home folder and `/` aren't projects.
- **The first time**, portcheck picks a port in `--range` (default `3000-3999`), starting
  from a hash of the project's path, so the same project tends to get the same port on
  every machine. It skips ports that are in use right now and ports that belong to another
  project.
- **After that**, the project keeps its port. If something else holds it at the moment,
  `portcheck port` still prints it and adds a note on stderr saying who holds it;
  `portcheck run --port auto` explains it and stops, or takes the next free port for that
  run only with `--next`, or stops the holder with `--kill`.

`--range` only applies to new assignments.

## See and change assignments

```console
$ portcheck port --list
PORT  PROJECT  PATH                             LAST USED
3456  shop     /Users/alice/code/shop           2026-11-09
3812  api      /Users/alice/code/api            2026-11-08
3901  old      /Users/alice/code/old (missing)  2026-09-01
```

| Command | Effect |
|---|---|
| `portcheck port --set 3005` | gives this project port 3005 (refused if another project has it; `--force` takes it over) |
| `portcheck port --forget` | removes this project's port; the next `portcheck port` assigns a new one |

`(missing)` marks projects whose folder is gone; `--forget` them from inside the folder,
or leave them, they only reserve a port.

## Where assignments are kept

In `ports.json` in portcheck's configuration folder: `~/Library/Application Support/portcheck`
on macOS, `~/.config/portcheck` on Linux, `%AppData%\portcheck` on Windows. Set
`PORTCHECK_CONFIG_DIR` to keep it somewhere else. Several `portcheck port` commands at once
are safe: updates take a lock and replace the file atomically. If the file is ever
damaged, portcheck says so, keeps it as `ports.json.corrupt` and starts a new one.

## `port` or `.portcheck.yaml`?

They solve different problems and work together:

- **`portcheck port`** is per machine and automatic: nothing to commit, and every project on
  your machine gets a different port.
- **[`.portcheck.yaml`](doctor.md)** is shared with your team in the repository: fixed,
  named ports (`portcheck run --port web`) that everyone uses. `auto` is reserved there,
  because `--port auto` means the project's own port.

## JSON

`portcheck port --json` prints the port, project name and path, and whether this call
assigned it; `--list --json` prints every assignment. See [JSON output](../json-schema.md#port).
