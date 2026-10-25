---
title: "Kill the process on a port on Windows (port already in use)"
description: "Find which process is using a port on Windows and stop it: with portcheck in one command, or with netstat and taskkill or PowerShell. Covers WinError 10048 and 10013."
---

# Kill the process using a port on Windows

## The quick way

In PowerShell or Command Prompt:

```powershell
npx portcheck-cli kill 3000
```

portcheck lists what is listening on port 3000 and who started it, asks before stopping
it, and refuses Windows system processes such as `svchost.exe`, `lsass.exe` and
`csrss.exe`. To install it permanently, use Scoop, npm (`npm install -g portcheck-cli`)
or the zip from the releases page: see [Install](../install.md).

To see what's there without stopping it:

```console
> portcheck 3000
PORT  PROTO  STATUS  PID    PROCESS   USER
3000  tcp    in-use  15872  node.exe  DESKTOP\alice
```

## With built-in tools

=== "Command Prompt (netstat, taskkill)"

    ```bat
    netstat -ano | findstr :3000
    ```

    The last column is the PID. Look up its name, then stop it:

    ```bat
    tasklist /FI "PID eq 15872"
    taskkill /PID 15872 /F
    ```

=== "PowerShell"

    ```powershell
    Get-NetTCPConnection -LocalPort 3000 -State Listen |
      Select-Object LocalPort, OwningProcess, @{n='Name';e={(Get-Process -Id $_.OwningProcess).Name}}
    Stop-Process -Id 15872
    ```

`findstr :3000` also matches ports like `30000` and connections *to* port 3000, so check
the PID before killing it. `taskkill /F` ends the program immediately, with no chance to
save its work.

!!! note "Access denied"
    Processes started by another user or as Administrator can only be stopped from an
    **Administrator** terminal: right-click PowerShell and choose **Run as
    administrator**.

## The errors

| Error | Meaning |
|---|---|
| `[WinError 10048] Only one usage of each socket address (protocol/network address/port) is normally permitted` | another program is listening: stop it as above |
| `[WinError 10013] An attempt was made to access a socket in a way forbidden by its access permissions` | usually a **reserved** port, see below |

## A port is "forbidden" but nothing uses it

Hyper-V, WSL2 and Docker reserve ranges of ports on Windows. Nothing is listening, yet
binding fails. portcheck reports these ports as `denied`. List the reservations:

```powershell
netsh interface ipv4 show excludedportrange protocol=tcp
```

Pick a port outside these ranges, or restart the NAT service as Administrator, which
often releases them: `net stop winnat; net start winnat`.

## WSL and Docker Desktop

A port held by `wslrelay.exe` belongs to a program inside WSL, and one held by
`com.docker.backend` belongs to a container. portcheck names the distro or container and
tells you how to stop it, instead of killing the relay. See
[Containers & WSL](../guide/containers.md).

## Related

- [Node.js, Next.js, Vite, React](node.md) · [Python](python.md) · [Docker](docker.md)
- [Port 80 held by System (PID 4)](web-servers.md#windows-port-80-held-by-system-pid-4) · [Databases](databases.md)
- [All "port already in use" fixes](index.md)
