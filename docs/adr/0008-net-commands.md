# ADR-0008: `net`: local network facts, opt-in network access

- Status: accepted
- Date: 2026-11-10

## Context

Users asked portcheck to show their IP addresses, find devices on their network and look
up names and addresses. These are close to portcheck's purpose (ports are reached through
addresses) but touch privacy and safety in ways port inspection doesn't: the public IP
needs a third-party service, and finding devices can mean sending traffic to other
people's machines.

## Decision

- **Local by default.** `portcheck net`, `net devices` and `net lookup` read only this
  machine: the interface list, the routing table, the neighbour (ARP) cache and the
  resolver configuration. The primary address is found by asking the OS which route a UDP
  "connection" to 1.1.1.1 would use; connecting UDP sends no packet.
- **One opt-in request to the internet.** Only `net --public` contacts a third party:
  one HTTPS GET each to `api.ipify.org` and `api6.ipify.org`, with a 5-second timeout. The
  endpoint is documented (`netinfo.PublicIPEndpoints`, the guide and the privacy page),
  and nothing else in the package makes requests.
- **The system resolver only.** `net lookup` and device names use the OS resolver, with
  no third-party DNS or IP-information APIs, so answers match what the machine sees and
  no query leaves through an unexpected path.
- **Probing is opt-in and bounded.** `net devices --probe` is the only active part. It
  reuses the TCP-connect scanner (ADR-0002: no raw sockets, no root), probes only
  private (RFC 1918) and link-local IPv4 networks, refuses networks larger than 1024 hosts
  unless `--max-hosts` raises the limit, rate-limits connections, prints a notice before
  it starts, and stops on Ctrl-C. IPv6 networks are never probed (a /64 can't be
  enumerated).
- **No shell, no cgo, no root.** OS facts come from procfs on Linux, from the routing
  socket (`golang.org/x/net/route`) and `route(8)` on macOS, and from `route.exe` and
  `arp.exe` on Windows, run directly (never through a shell) with the C locale. Each OS
  only lists its sources; parsing and logic are shared and tested on every platform.
- **Vendor hints are small and honest.** A short table of common vendors' OUIs from the
  IEEE registry, labelled a hint; locally administered (randomised) MACs are named as
  such rather than guessed.

## Consequences

- No new services or keys; `net` works offline except for `--public`.
- macOS 15 and later can hide neighbour-cache entries from processes without Local
  Network permission (entries for hosts the process contacted itself stay visible);
  `net devices` then says how to allow it, and `--probe` still finds devices.
- Windows shows no DNS servers, and only IPv4 neighbours are listed on every OS.
- The vendor table covers few vendors on purpose; unknown devices show no vendor rather
  than a wrong one.
