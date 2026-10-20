# Security Policy

## Supported versions

| Version | Supported |
|---|---|
| latest minor release (`0.x`) | ✅ |
| older releases | ❌ — please upgrade |

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Use GitHub's private vulnerability reporting:
<https://github.com/roystondwork-dev/portcheck/security/advisories/new>

Include as much as you can:

- affected version (`portcheck version --json`) and OS,
- a description of the issue and its impact,
- steps or a proof of concept to reproduce it.

You can expect:

- an acknowledgement within **3 business days**,
- an initial assessment within **10 business days**,
- coordinated disclosure: we will agree on a release date with you and credit
  you in the advisory unless you prefer to remain anonymous.

## Scope notes

portcheck inspects local sockets, can terminate processes on request, and can
perform TCP connect checks against remote hosts. Issues we especially want to
hear about:

- ways to make `portcheck kill` terminate a process other than the one the user
  confirmed, or bypass its safety guards;
- privilege escalation when portcheck is run with `sudo` / as Administrator;
- injection via process names, command lines or hostnames rendered in the TUI or output.

## Verifying releases

Release artifacts are signed with [cosign](https://github.com/sigstore/cosign)
(keyless) and ship with an SBOM and SLSA build provenance. See
[docs/install.md](docs/install.md#verifying-downloads) for verification commands.

## Responsible use

`portcheck scan` performs ordinary TCP connections. Only scan hosts and networks
you own or are explicitly authorized to test.
