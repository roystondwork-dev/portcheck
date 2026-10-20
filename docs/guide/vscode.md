# VS Code extension

The portcheck extension shows what's using your ports and frees them safely, without
leaving the editor. It works in VS Code and in editors that use Open VSX (VSCodium,
Cursor, Windsurf).

!!! note "Status"
    The extension is in development (milestones M14–M16 in
    [TASKS.md](https://github.com/roystondwork-dev/portcheck/blob/main/TASKS.md)). Until
    it's published on the Marketplace, you can build and install it from source
    (see [Building from source](#building-from-source)).

## Listening ports view

The **portcheck** icon in the activity bar opens **Listening ports**: one row per
listening port and owner.

| Row | Meaning |
|---|---|
| `:3000  node` (process icon) | a process you can stop |
| `:8080  docker:web` (package icon) | a port published by a Docker/Podman container, or forwarded from a WSL distro (`wsl:Ubuntu/node`) |
| `:22  (owner hidden)` (lock icon) | the owner belongs to another user and is hidden without elevated privileges |

Hover a row for details: the process, **who started it** (`npm (4200) ← zsh (900)`),
the command line, the user, and for containers the image and the command that stops it.

The view refreshes every 5 seconds while it's visible (setting
`portcheck.refreshInterval`; `0` refreshes only on demand) and has a refresh button.

## Stopping a process

Use the stop button on a row, or **portcheck: Stop What's Using a Port…** from the
command palette.

- For a **process**, you're asked first, and the dialog shows who started it. **Stop**
  asks it to exit and forces it only if needed; **Force Kill** skips the graceful step.
  portcheck's [safety guards](kill.md#safety) apply: it refuses system processes and
  never stops VS Code itself.
- For a **container or WSL port**, the extension never stops the proxy, because that
  would affect every container or the whole distro. It shows the right command
  (`docker stop web`, `wsl -d Ubuntu --exec kill 1234`) and can run it in a terminal
  or copy it.
- For a **hidden owner**, it offers the elevated command (`sudo portcheck kill 22`) to
  run in a terminal.

## Commands

| Command | What it does |
|---|---|
| **portcheck: Check a Port…** | whether a port is free, and who holds it if not (with a **Stop It** action) |
| **portcheck: Find a Free Port…** | the first free port in a range (default `3000-9999`); **Copy** or **Insert** it at the cursor |
| **portcheck: Stop What's Using a Port…** | as above, by port number |
| **portcheck: Refresh** | reloads the view |
| **portcheck: Show Log** | the extension's log, useful when reporting issues |

## Remote development

The extension runs where your code runs. In **Remote-SSH**, **WSL** and **Dev
Containers** it lists and stops ports on the remote machine, distro or container, not
on your laptop.

## Settings

| Setting | Default | Meaning |
|---|---|---|
| `portcheck.path` | *(empty)* | path to the `portcheck` binary; empty uses the bundled one, then `portcheck` on your `PATH` |
| `portcheck.refreshInterval` | `5` | seconds between refreshes while the view is visible; `0` = manual |

## How it works

The extension is a thin client: every action runs the `portcheck` CLI with `--json`
([versioned schema](../json-schema.md)) and shows the result. Process attribution,
containers, WSL and the safety guards all come from the same code as the CLI. See
[ADR-0004](../adr/0004-vscode-extension.md).

## Building from source

Requires Go 1.26+ and Node.js 22+.

```sh
git clone https://github.com/roystondwork-dev/portcheck.git && cd portcheck
make build                         # builds bin/portcheck
cd extensions/vscode
npm ci
npx vsce package --no-dependencies # creates portcheck-<version>.vsix
code --install-extension portcheck-*.vsix
```

Then set **portcheck.path** to the absolute path of `bin/portcheck` (or put it on your
`PATH`).
