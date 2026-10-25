---
title: "Port 80, 443 or 8080 already in use (nginx, Apache, Caddy, IIS)"
description: "Fix “bind() to 0.0.0.0:80 failed (Address already in use)” in nginx, Apache and Caddy, and port 80 held by System (PID 4) on Windows: find what holds the port and stop it."
---

# Port 80, 443 or 8080 already in use (nginx, Apache, Caddy)

| Server | You see |
|---|---|
| nginx | `nginx: [emerg] bind() to 0.0.0.0:80 failed (98: Address already in use)` (`48:` on macOS) |
| Apache httpd | `(98)Address already in use: AH00072: make_sock: could not bind to address [::]:80`, then `no listening sockets available, shutting down` |
| Caddy | `listen tcp :80: bind: address already in use` |

Usually another web server already has the port: a second nginx, an Apache that the
OS started, a container, or on Windows a system service.

## Find out what holds it

```sh
npx portcheck-cli 80 443 8080
```

Web servers on port 80 usually run as root or a service user, so without privileges
their process shows as hidden. Then install portcheck (see [Install](../install.md))
and run `sudo portcheck 80`; on Windows, use an **Administrator** terminal.

Then stop it. For a stray process, `portcheck kill 80` asks and stops it. For a web
server that runs as a service, stop the service, or it starts again:

=== "macOS"

    ```sh
    brew services stop nginx          # Homebrew nginx, httpd or caddy
    sudo apachectl stop               # Apple's built-in Apache, if you enabled it
    ```

=== "Linux"

    ```sh
    sudo systemctl stop nginx         # or apache2 (Debian/Ubuntu), httpd (Fedora), caddy
    sudo systemctl disable nginx      # if it shouldn't start at boot
    ```

=== "Docker"

    portcheck names the container (`docker:web`) and `portcheck kill 80` prints
    `docker stop web` for it. See [Docker](docker.md).

## Windows: port 80 held by System (PID 4)

On Windows, `portcheck 80` often shows the holder as `System`, PID 4. That's the
kernel's HTTP service (http.sys), which serves port 80 and 443 on behalf of other
programs: **IIS** (the "World Wide Web Publishing Service"), SQL Server Reporting
Services, the Web Deployment Agent Service and others.

portcheck refuses to stop it (`refusing to kill protected process`): `System` is on
its list of processes whose loss would crash Windows. Find the real owner instead, in
an Administrator terminal:

```powershell
netsh http show servicestate
```

The output lists each registered URL with the process that owns it. Stop that
program's service in **Services** (`services.msc`), for IIS:

```powershell
net stop W3SVC
```

## Ports below 1024 need privileges

Ports such as 80 and 443 are special:

- **Linux:** binding them needs root, or `CAP_NET_BIND_SERVICE`.
- **macOS:** any user may bind them on all addresses (`0.0.0.0`), but not on
  `127.0.0.1` alone.

So `portcheck check 80` can say `denied` even when nothing listens: the port is free,
your user just can't bind it. For local development, a port above 1024 such as 8080
avoids this; see [Troubleshooting](../troubleshooting.md).

## Port 8080

8080 is the default of many tools, Jenkins and Tomcat among them, plus Spring Boot and
countless dev servers. `portcheck 8080` names the one you have; if you need both, move
one: `portcheck free 8080-8099` lists free ports nearby.

## Related

- [Rails, Spring Boot, Go, .NET, PHP](others.md) · [Docker](docker.md)
- [macOS](macos.md) · [Windows](windows.md) · [Linux](linux.md)
- [All "port already in use" fixes](index.md)
