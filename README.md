# portcheck

> See what's using your ports, find free ones, free them up, and check remote hosts. Works on macOS, Linux and Windows.

[![CI](https://github.com/roystondwork-dev/portcheck/actions/workflows/ci.yml/badge.svg)](https://github.com/roystondwork-dev/portcheck/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> 🚧 **Status: pre-release, under active development.** See [TASKS.md](TASKS.md) for progress.
> Install from source until the first release: `go install github.com/roystondwork-dev/portcheck/cmd/portcheck@latest`

## Planned features

- `portcheck list`: all open ports with their owning process (PID, name, user).
- `portcheck check 3000`: is the port free? If not, shows who holds it.
- `portcheck free`: finds the next free port.
- `portcheck kill 3000`: safely stops the process holding a port.
- `portcheck scan host -p 1-1024`: concurrent TCP connect checks, for hosts you're authorized to test.
- `portcheck tui`: a live, interactive terminal UI.
- `--json` output and meaningful exit codes for scripts and CI.
- A reusable Go library under `pkg/`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Please read our [Code of Conduct](CODE_OF_CONDUCT.md)
and report security issues as described in [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © 2026 Royston D and portcheck contributors
