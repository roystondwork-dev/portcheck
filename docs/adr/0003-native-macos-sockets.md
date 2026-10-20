# ADR-0003: Read macOS sockets through libproc instead of running lsof

- **Status:** Accepted (amends [ADR-0001](0001-gopsutil-provider.md) for macOS)
- **Date:** 2026-10-20

## Context

On macOS, gopsutil lists sockets by running `lsof` and parsing its text output
(ADR-0001). That works, but it starts a process for every listing (every 2 seconds
in the TUI), depends on `lsof`'s output format, and drops some entries it can't
parse.

`lsof` itself reads sockets through libproc in libSystem: `proc_listallpids`,
`proc_pidinfo(PROC_PIDLISTFDS)` and `proc_pidfdinfo(PROC_PIDFDSOCKETINFO)`. The
structures are declared in the public `<sys/proc_info.h>`.

Calling libSystem normally requires cgo, which would break our release builds:
macOS binaries are cross-compiled with `CGO_ENABLED=0`.

## Decision

- Call the three libproc functions directly with
  [purego](https://github.com/ebitengine/purego), which loads C functions without cgo.
  It is already a dependency through gopsutil.
- Decode `struct socket_fdinfo` in portable Go, using offsets taken from the macOS SDK.
  They are identical on arm64 and x86_64, and a test re-checks them with clang against
  the SDK on every macOS CI run.
- Keep the gopsutil/`lsof` path as an automatic fallback if the native call fails.
- Keep gopsutil for everything else (process names, users, command lines, other OSes).

## Consequences

- ✅ No `lsof` process per listing. Warm `portcheck list` went from about 20 ms to
  under 10 ms; the socket listing itself from about 43 ms to 4 ms.
- ✅ Same visibility as `lsof`: own processes without root, all processes as root.
  A live-system integration test asserts both report the same listening sockets.
- ✅ Still `CGO_ENABLED=0`, so cross-compiled releases are unchanged.
- ⚠️ We depend on the layout of `struct socket_fdinfo`. It is part of Apple's public
  ABI and has been stable for years; the SDK layout test would catch a change, and the
  `lsof` fallback limits the blast radius.
- ⚠️ Pointers passed through purego must be `unsafe.Pointer` to Go buffers, never
  `uintptr`, or the kernel can write into a stale copy. The call sites are commented
  and audited.
