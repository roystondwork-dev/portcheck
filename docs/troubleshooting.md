# Troubleshooting

Run any command with `-v` to log diagnostics to stderr.

## A port is in use but portcheck shows no process

The owner belongs to another user and your user can't see it. Re-run with
`sudo` or as Administrator; see [Permissions by OS](permissions.md). On macOS without `sudo`,
the socket may not appear in `list` at all, but `check` still reports the port as
`in-use` because binding it fails.

## A container's port shows `com.docker.backend` or `docker-proxy`

portcheck couldn't reach the container engine, so it shows the proxy process. Check
that the engine is running (`docker ps`), and that your user can access its socket
(on Linux: the `docker` group). If you use a non-default endpoint or a Docker
context, set `DOCKER_HOST`. See [Containers](guide/containers.md).

## A port shows `wslrelay.exe` or `wslservice.exe` (Windows)

portcheck couldn't tell which WSL distro is behind the port. Check that the distro is
running (`wsl --list --running`) and that the server is listening there
(`wsl -d <distro> --exec ss -ltn`). The lookup gives up after 3 seconds, which a cold
WSL VM can exceed; run the command again once WSL is up. See
[Containers & WSL](guide/containers.md#wsl-windows).

## Something looks different on macOS than `lsof -i`

portcheck reads the same kernel data as `lsof` but without running it. It hides
sockets not bound to any port (`lsof`'s `*:*` UDP entries). Otherwise the listings
should match; if they don't, please open an issue with both outputs.

## `check` says `denied` for a port below 1024

The port may be free, but **you** can't bind it on every address:

- **Linux:** ports below 1024 need root (or `CAP_NET_BIND_SERVICE`).
- **macOS:** any user may bind them on `0.0.0.0`/`::`, but not on `127.0.0.1`/`::1`.

Pick a port ≥ 1024, or run as root.

## Windows says a port is `denied` but nothing uses it

Windows reserves port ranges for Hyper-V, WSL2 and Docker (WinNAT). Binding a reserved
port fails with "access forbidden". List the reservations:

```powershell
netsh interface ipv4 show excludedportrange protocol=tcp
```

Pick a port outside these ranges. Restarting the `winnat` service
(`net stop winnat; net start winnat`, as Administrator) often frees dynamic
reservations.

## `kill` says "pass --yes"

stdin is not a terminal (a pipe, CI, cron, an IDE task runner), so portcheck can't ask
for confirmation. Add `--yes` once you're sure about the target; `--dry-run` previews it.

## `kill` says "refusing to kill protected process"

The target is PID 0/1, portcheck itself, your shell (portcheck's parent), or a
well-known system process. The first three are never allowed. For system processes,
`--allow-system` overrides the guard, but stopping them can crash your session or OS.

## A process survives `kill` (exit code 3)

After `SIGKILL`, a process that doesn't exit is usually stuck in the kernel (for
example, uninterruptible I/O on a dead network mount) or is a zombie whose parent
hasn't reaped it. Restarting its parent, or the machine, is the remedy.

## `scan` shows everything as `filtered`

Nothing answered within `--timeout`. Either a firewall drops the packets, or the
network is slow. Try `--timeout 3s --retries 1`. If every port, including ones you
know are open, shows `filtered`, check basic reachability (`ping`, VPN, routes).

## `scan` is slow

The rate limit (`--rate 500` connections/s) and `--concurrency 100` keep scans polite.
On your own network you can raise them, e.g. `--rate 0 --concurrency 500`. Filtered
ports always take the full `--timeout`.

## `portcheck` alone prints help instead of opening the TUI

The TUI opens only when both stdin and stdout are terminals. Use `portcheck tui` in a
real terminal; in pipes and CI, portcheck prints help on purpose.

## Colors look wrong or appear in files

Colors are only used when stdout is a terminal. Set `NO_COLOR=1`, or pass `--no-color`,
to turn them off in command output and in the TUI.

## Still stuck?

Open an [issue](https://github.com/roystondwork-dev/portcheck/issues/new/choose) and
include `portcheck version --json`, your OS, whether you used `sudo`/Administrator,
and the output of the failing command with `-v`.
