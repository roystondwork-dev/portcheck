# ADR-0001: Use gopsutil behind a Provider interface for socket/process data

- **Status:** Accepted
- **Date:** 2026-10-20

## Context

portcheck must map local sockets to owning processes on macOS, Linux and
Windows. Each OS exposes this differently:

| OS | Native source |
|---|---|
| Linux | `/proc/net/{tcp,tcp6,udp,udp6}` + `/proc/<pid>/fd` inode matching |
| macOS | `libproc` (`proc_pidinfo`, needs cgo) or `lsof` |
| Windows | `GetExtendedTcpTable` / `GetExtendedUdpTable` (iphlpapi) |

Options considered:

1. **Shell out to `lsof` / `netstat` / `ss`** — fragile text parsing, output differs by OS version and locale.
2. **Write native implementations per OS** — best control, but large surface, and macOS needs cgo, which breaks easy cross-compilation.
3. **Use `github.com/shirou/gopsutil/v4`** — mature, widely used, pure Go on Linux/Windows; on macOS it uses `lsof` internally.

## Decision

Use gopsutil (option 3), **but only behind our own `inspect.Provider` interface**:

```go
type Provider interface {
    Sockets(ctx context.Context) ([]RawSocket, error)
    Process(ctx context.Context, pid int32) (*ProcessInfo, error)
}
```

All enrichment, filtering, de-duplication and sorting happen in portcheck's own
code against this interface, and are tested with a deterministic `FakeProvider`.

## Consequences

- ✅ Works on all three OSes with `CGO_ENABLED=0` static builds.
- ✅ Business logic is fully unit-testable without real sockets.
- ✅ A native provider (e.g. libproc on macOS) can be added later without touching callers.
- ⚠️ On macOS, results depend on `lsof` (preinstalled on every macOS) and, without
  `sudo`, only show processes of the current user. We detect this and print a hint.
- ⚠️ gopsutil is a third-party dependency; Dependabot + govulncheck track it.
