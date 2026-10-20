# Permissions by OS

Operating systems restrict what an ordinary user can see about other users'
processes. portcheck never hides this: rows whose owner it cannot see show PID `-`
and process `?` (or a PID with process `?` when only the process details are
denied). In JSON, `pid` or `process` is omitted accordingly and `partial` is
`true`. A one-line hint is printed to stderr.

| | macOS | Linux | Windows |
|---|---|---|---|
| Sockets of other users | **hidden** | listed | listed |
| PID / process of other users' sockets | hidden | **hidden** | PID listed; name/user may be hidden |
| Stop other users' processes | needs `sudo` | needs `sudo` | needs Administrator |
| Bind ports below 1024 (`check`/`free`) | wildcard only² | needs root¹ | allowed |
| `scan` | no privileges needed | no privileges needed | no privileges needed |

¹ Unless `net.ipv4.ip_unprivileged_port_start` has been lowered, or the binary has `CAP_NET_BIND_SERVICE`.  
² macOS lets any user bind low ports on `0.0.0.0` and `::`, but binding a specific address such as `127.0.0.1` needs root.

## macOS

portcheck reads sockets through `lsof`, which is part of every macOS install. Without
root, `lsof` only reports your own processes, so other users' sockets are **missing**,
not just anonymous. `portcheck check` still detects that such a port is busy, because
it also tries to bind it, but cannot name the owner.

```sh
sudo "$(command -v portcheck)" list -l
```

Ports below 1024 can be bound by any user on the wildcard addresses (`0.0.0.0`,
`::`), but not on specific addresses like `127.0.0.1`. Because `check` and `free`
test every address, they report such ports as `denied` without root.

## Linux

portcheck reads `/proc/net/{tcp,tcp6,udp,udp6}`, which lists every socket on the
system, and maps sockets to processes through `/proc/<pid>/fd`. Only root can read
another user's `/proc/<pid>/fd`, so those sockets appear with PID `-`.

```sh
sudo "$(command -v portcheck)" list -l
```

In containers, portcheck sees the container's network namespace and processes, not
the host's.

## Windows

portcheck uses the IP Helper API (`GetExtendedTcpTable` / `GetExtendedUdpTable`),
which reports the owning PID of every socket. Process names and users of services
and other sessions may need an elevated terminal. Right-click your terminal and
choose **Run as administrator**.

Windows may reserve port ranges for Hyper-V, WSL or Docker. Binding them fails even
though nothing is listening, so `check` reports them as `denied`. See
[Troubleshooting](troubleshooting.md#windows-says-a-port-is-denied-but-nothing-uses-it).

## Is running as root safe?

portcheck only reads system information, except for `kill`, which always asks
before acting (or requires `--yes`) and refuses system processes. It writes no
files and makes no network connections, except `scan` to the targets you name.
