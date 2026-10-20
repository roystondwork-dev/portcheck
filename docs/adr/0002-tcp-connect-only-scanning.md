# ADR-0002: Remote scanning uses TCP connect only

- **Status:** Accepted
- **Date:** 2026-10-20

## Context

`portcheck scan` checks whether ports are reachable on remote hosts. Scanners
such as nmap support many techniques (SYN "half-open", FIN/NULL/Xmas, UDP,
fragmentation, decoys, timing evasion). Most require raw sockets (root /
Administrator, Npcap on Windows) and several exist mainly to evade detection.

portcheck's audience is developers and operators checking their **own**
services ("is my DB port reachable from here?").

## Decision

- Use the operating system's normal `connect()` via Go's `net.Dialer` only.
- Classify results as `open` (connected), `closed` (connection refused),
  `filtered` (timeout / no response) and `error` (DNS failure, unreachable, …).
- Provide safe defaults: bounded concurrency (100), per-connection timeout (1s),
  a global rate limit (500 connections/s) and a cap on CIDR expansion (1024 hosts).
- **Do not** implement stealth, evasion, spoofing or decoy features.
- Docs and `--help` state that users must only scan systems they are authorized to test.

## Consequences

- ✅ No elevated privileges or extra drivers on any OS.
- ✅ Behaviour is identical across macOS, Linux and Windows.
- ✅ Easy to test with a fake `Dialer` and real loopback listeners.
- ⚠️ Connect scans are logged by targets and slower than SYN scans; this is acceptable and intended.
- ⚠️ UDP scanning is out of scope for now (unreliable without protocol-specific probes).
