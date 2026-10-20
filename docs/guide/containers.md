# Containers (Docker & Podman)

A port published by a container (`docker run -p 8080:80 …`) is held on your machine by
a proxy process: `com.docker.backend` with Docker Desktop, `docker-proxy` on Linux,
and so on. Showing that process would be accurate but not useful, so when a
container engine is reachable, portcheck shows the **container** instead:

```console
$ portcheck list -l --port 8080
PROTO  LOCAL ADDRESS  REMOTE ADDRESS  STATE   PID    PROCESS     USER
tcp    [::]:8080      -               LISTEN  79156  docker:web  alice
```

This works in `list`, `check`, the [TUI](tui.md) and JSON output (a `container` object
with `id`, `name`, `image` and `runtime`; see [JSON output](../json-schema.md#shared-objects)).
`--name` matches container names too: `portcheck list --name 'web*'`.

## Stopping a container's port

`kill` never stops the proxy. Doing so would take down Docker itself, or every
container it serves. Instead it tells you which container to stop:

```console
$ portcheck kill 8080
Port 8080 is published by docker container "web" (nginx:alpine). Stop the container instead:
  docker stop web
```

The exit status is `1`, and other, non-container targets in the same command are
still handled. The TUI shows the same hint when you press ++x++ on a container row.
Docker, containerd, Podman networking and WSL infrastructure processes are also on
`kill`'s [protected list](kill.md#safety), so `kill --pid` refuses them too.

## Which engines are supported

Any engine with a Docker-compatible API:

| Engine | Endpoint portcheck tries |
|---|---|
| Docker Desktop (macOS) | `~/.docker/run/docker.sock`, then `/var/run/docker.sock` |
| Colima / OrbStack (macOS) | `~/.colima/default/docker.sock`, `~/.orbstack/run/docker.sock` |
| Podman machine (macOS) | `~/.local/share/containers/podman/machine/podman.sock` |
| Docker Engine (Linux) | `/var/run/docker.sock` |
| Rootless Docker / Podman (Linux) | `$XDG_RUNTIME_DIR/docker.sock`, `$XDG_RUNTIME_DIR/podman/podman.sock`, `/run/podman/podman.sock` |
| Docker Desktop / Podman (Windows) | named pipes `docker_engine`, `podman-machine-default` |

**`DOCKER_HOST` takes precedence** when it's set. Supported forms are
`unix:///path`, `npipe:////./pipe/name` and `tcp://host:port`. TLS (`tcp://` with
certificates) and `ssh://` endpoints aren't supported; with those, portcheck skips
container lookup. Docker *contexts* are not read; set `DOCKER_HOST` if you use one.

## Cost and failure behaviour

The lookup is a single local API call, typically around 10 ms. It's limited to
500 ms, and if the engine isn't running, isn't reachable, or you lack access to its
socket, portcheck silently skips it and shows the proxy process as before. A
container engine never makes portcheck slow or fail.

## Limitations

- **Linux without the userland proxy.** If Docker runs with
  `"userland-proxy": false`, published ports are forwarded by firewall rules and no
  process listens on them. portcheck can't see such ports, and `check`/`free` may
  report them as free.
- **Socket permissions on Linux.** Reading `/var/run/docker.sock` needs membership in
  the `docker` group (or root). Without it, container attribution is skipped.
- **WSL.** On Windows, ports forwarded from a WSL distro show `wslrelay.exe`. It's
  protected from `kill`, but the distro isn't identified yet.
