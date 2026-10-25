---
title: "Port 5432, 3306, 6379 or 27017 already in use (PostgreSQL, MySQL, Redis, MongoDB)"
description: "Fix “Address already in use” for PostgreSQL 5432, MySQL and MariaDB 3306, Redis 6379 and MongoDB 27017: find the service or container holding the port and stop it the right way."
---

# Database port already in use (PostgreSQL, MySQL, Redis, MongoDB)

| Database | Port | You see |
|---|---|---|
| PostgreSQL | 5432 | `could not bind IPv4 address "127.0.0.1": Address already in use`, then `Is another postmaster already running on port 5432?` |
| MySQL / MariaDB | 3306 | `Bind on TCP/IP port: Address already in use` and `Do you already have another mysqld server running on port: 3306 ?` |
| Redis | 6379 | `Could not create server TCP listening socket *:6379: bind: Address already in use` |
| MongoDB | 27017 | a log line with `Error setting up listener` and `Address already in use` |
| any of them in Docker | | `Bind for 0.0.0.0:5432 failed: port is already allocated` |

Almost always, **another copy of the same database is already running**: one installed
with Homebrew, apt or an installer that starts at login, a desktop app such as
Postgres.app, or a container from another project.

## Find out what holds it

```sh
npx portcheck-cli 5432
```

```console
PORT  PROTO  STATUS  PID  PROCESS   USER
5432  tcp    in-use  812  postgres  alice
```

A container shows as `docker:<name>` instead, and `portcheck kill 5432` prints the
`docker stop` command for it rather than stopping Docker's proxy. Run as another user
(Linux packages run databases as `postgres`, `mysql` or `redis`), the process may show
as hidden: use `sudo portcheck 5432`.

## Stop the service, not just the process

A database started as a service comes straight back if you only kill its process.
Stop the service instead:

=== "macOS (Homebrew)"

    ```sh
    brew services list                 # which databases Homebrew runs
    brew services stop postgresql@17   # use the name from the list
    brew services stop mysql
    brew services stop redis
    brew services stop mongodb-community
    ```

    Postgres.app: quit it from its menu bar icon.

=== "Linux (systemd)"

    ```sh
    systemctl list-units --type=service | grep -Ei 'postgres|mysql|maria|redis|mongo'
    sudo systemctl stop postgresql
    sudo systemctl stop mysql          # or mariadb
    sudo systemctl stop redis-server   # Debian/Ubuntu; redis on Fedora
    sudo systemctl stop mongod
    ```

    Add `sudo systemctl disable <name>` to keep it from starting at boot.

=== "Windows"

    Open **Services** (`services.msc`), find the database (for example
    `postgresql-x64-17` or `MySQL80`), and click **Stop**. Set **Startup type** to
    **Manual** to keep it from starting with Windows.

=== "Docker"

    ```sh
    docker ps --filter publish=5432
    docker stop <container>            # or docker compose down in its project
    ```

If it's a stray process rather than a service, `npx portcheck-cli kill 5432` stops it,
asking first and shutting it down gently so the database can close its files.

## Or run the second copy on another port

Often you want both, for example a local PostgreSQL and a project's container. Publish
the container on another host port and point your app at it:

```sh
docker run -p 5433:5432 postgres:17
```

```yaml
# docker-compose.yml
services:
  db:
    image: postgres:17
    ports:
      - "5433:5432"
```

`portcheck free 5433-5499` lists free ports to choose from.

## Related

- [Docker: port is already allocated](docker.md)
- [macOS](macos.md) · [Windows](windows.md) · [Linux](linux.md)
- [All "port already in use" fixes](index.md)
