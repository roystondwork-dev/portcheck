---
title: "Docker: port is already allocated / address already in use"
description: "Fix “Bind for 0.0.0.0:8080 failed: port is already allocated” and “bind: address already in use” in Docker and Docker Compose: find the container or process holding the port."
---

# Docker: port is already allocated

```text
Error response from daemon: driver failed programming external connectivity on endpoint web:
Bind for 0.0.0.0:8080 failed: port is already allocated
```

```text
Error starting userland proxy: listen tcp4 0.0.0.0:5432: bind: address already in use
```

Both mean the host port you're publishing (`-p 8080:80`, or `ports:` in Compose) is
taken. The first usually means **another container** already publishes it; the second
usually means **a program on your machine** does (often a locally installed Postgres,
Redis or MySQL).

## Find out which one

```sh
npx portcheck-cli 8080
```

portcheck recognizes container ports and names the container rather than showing
`com.docker.backend` or `docker-proxy`:

```console
$ portcheck kill 8080
Port 8080 is published by docker container "web" (nginx:alpine). Stop the container instead:
  docker stop web
```

It never kills Docker's own proxy process, which would take down Docker or every
container it serves.

## If it's another container

```sh
docker ps --filter publish=8080        # which container
docker stop web                        # or: docker compose down, in its project
```

A common cause is a Compose project started from another folder, or an old project
whose containers were never stopped: `docker ps` shows them all.

## If it's a program on your machine

A database or dev server installed directly on your machine:

```sh
npx portcheck-cli kill 5432
```

For databases run as services, stop the service instead of the process, or it will
start again (more in [Databases](databases.md)): `brew services stop postgresql@17` on macOS,
`sudo systemctl stop postgresql` on Linux. On Windows, use the **Services** app.

Or publish the container on another host port: `-p 5433:5432`.

## Docker Desktop on Windows and WSL

Docker Desktop and WSL2 reserve ranges of ports on Windows. If nothing is listening but
the bind still fails, the port is probably reserved; see
[Windows: a port is "forbidden" but nothing uses it](windows.md#a-port-is-forbidden-but-nothing-uses-it).

## Related

- [Containers & WSL in portcheck](../guide/containers.md)
- [macOS](macos.md) · [Windows](windows.md) · [Linux](linux.md)
- [All "port already in use" fixes](index.md)
