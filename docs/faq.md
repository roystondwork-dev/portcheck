# FAQ

### How is this different from `lsof`, `netstat`, `ss` or `Get-NetTCPConnection`?

They are great, but each OS has different ones with different flags and output.
portcheck gives you the same commands, output, JSON schema and exit codes everywhere,
and adds what those tools don't: "is this port free, really?", "find me a free port",
safe kill, and a live TUI.

### Why does the same port appear twice?

A program listening on both IPv4 (`0.0.0.0:3000`) and IPv6 (`[::]:3000`) has two
sockets, so you see two rows.

### Is `scan` a replacement for nmap?

No. `scan` answers a simple question, "can I connect to these ports from here?", using
ordinary TCP connections. It has no OS or service detection, UDP scanning or
stealth techniques, by design ([ADR-0002](adr/0002-tcp-connect-only-scanning.md)).

### Does portcheck send telemetry or phone home?

No. It makes no network connections except the ones `scan` makes to the targets you
give it, and it writes no files.

### Why does `free` start at 1024?

Ports below 1024 usually need root to bind (on Linux), so they are rarely useful for
development. Pass a range to override: `portcheck free 80-1023`.

### Can I use portcheck from Go code?

Yes. Everything is built on public packages under `pkg/`; see [Go library](library.md).

### Which versions of each OS are supported?

Release binaries are built for amd64 and arm64 on macOS, Linux and Windows, and run
wherever the Go release they are built with runs; see Go's
[minimum requirements](https://go.dev/wiki/MinimumRequirements). CI tests every change
on current Ubuntu, macOS and Windows.

### Why is it called portcheck?

Because `portcheck 3000` is what you type.
