<h1 align="center">portcheck</h1>

<p align="center">
  <b>See what's using your ports, find free ones, free them up, and check remote hosts.</b><br>
  One static binary for macOS, Linux and Windows.
</p>

<p align="center">
  <a href="https://github.com/roystondwork-dev/portcheck/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/roystondwork-dev/portcheck/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/roystondwork-dev/portcheck/actions/workflows/codeql.yml"><img alt="CodeQL" src="https://github.com/roystondwork-dev/portcheck/actions/workflows/codeql.yml/badge.svg"></a>
  <a href="https://codecov.io/gh/roystondwork-dev/portcheck"><img alt="Coverage" src="https://codecov.io/gh/roystondwork-dev/portcheck/graph/badge.svg"></a>
  <a href="https://github.com/roystondwork-dev/portcheck/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/roystondwork-dev/portcheck"></a>
  <a href="https://scorecard.dev/viewer/?uri=github.com/roystondwork-dev/portcheck"><img alt="OpenSSF Scorecard" src="https://api.scorecard.dev/projects/github.com/roystondwork-dev/portcheck/badge"></a>
  <a href="https://pkg.go.dev/github.com/roystondwork-dev/portcheck"><img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/roystondwork-dev/portcheck.svg"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/License-MIT-blue.svg"></a>
</p>

<p align="center"><img alt="portcheck demo" src="docs/assets/demo.gif" width="800"></p>

## Features

| | Command | |
|---|---|---|
| 🔍 | `portcheck list -l` | Every listening socket with its process, PID and user |
| ✅ | `portcheck 3000` | Is the port free? If not, who holds it? Exit code 0 or 1 for scripts |
| 🆓 | `portcheck free 3000-3999 -n 3` | Free ports, verified by actually binding them |
| 🛑 | `portcheck kill 8080` | Graceful SIGTERM, then SIGKILL; refuses PID 1, your shell and system processes |
| 📡 | `portcheck scan db.internal -p 5432` | Rate-limited TCP connect checks of remote hosts. No root needed |
| ⏳ | `portcheck wait db:5432 -t 60s` | Block until services accept connections, or until a port is free. Built for CI |
| 🖥️ | `portcheck` | Live, sortable, filterable TUI. Stop processes without leaving it |

- **Same commands and output on every OS.** It replaces `lsof -i`, `netstat -ano`, `ss -ltnp` and `kill`.
- **Built for scripts.** `--json` follows a [versioned schema](docs/json-schema.md), [exit codes](docs/exit-codes.md) are stable, and results go to stdout while messages go to stderr.
- **Honest about permissions.** Hidden owners are shown as hidden, with a hint on how to see them.
- **A Go library too.** See [`pkg/`](https://pkg.go.dev/github.com/roystondwork-dev/portcheck).

## Install

```sh
# Homebrew (macOS, Linux)
brew install roystondwork-dev/tap/portcheck

# winget (Windows)
winget install roystondwork-dev.portcheck

# Scoop (Windows)
scoop bucket add portcheck https://github.com/roystondwork-dev/scoop-bucket
scoop install portcheck

# Go 1.26+
go install github.com/roystondwork-dev/portcheck/cmd/portcheck@latest
```

`.deb`, `.rpm`, `.apk` and archives for amd64/arm64 are on the
[releases page](https://github.com/roystondwork-dev/portcheck/releases). The
checksum file covering every artifact is signed with cosign (keyless), and
releases ship SBOMs and SLSA provenance; see
[verifying downloads](docs/install.md#verifying-downloads).

## Quickstart

```console
$ portcheck 3000
PORT  PROTO  STATUS  PID    PROCESS  USER
3000  tcp    in-use  42379  node     alice

$ portcheck kill 3000
About to terminate:
  PID 42379  node  (port 3000)
Proceed? [y/N] y
PID    PROCESS  RESULT      DETAIL
42379  node     terminated

$ portcheck free 3000-3999 -n 2 -o plain
PORT
3000
3001

$ portcheck list -l --json | jq '.data.sockets[] | {port: .localPort, name: .process.name}'
```

📖 **Documentation:** <https://roystondwork-dev.github.io/portcheck/>
([quickstart](docs/quickstart.md) · [guides](docs/guide/) · [CLI reference](docs/commands/portcheck.md) · [permissions by OS](docs/permissions.md) · [FAQ](docs/faq.md))

## Use as a Go library

```go
res, err := inspect.NewSystem().List(ctx, inspect.Filter{Ports: portspec.Single(3000), ListeningOnly: true})
if err != nil {
	log.Fatal(err)
}
for _, s := range res.Sockets {
	if s.Process != nil { // nil when the owner is hidden from your user
		fmt.Println(s.PID, s.Process.Name)
	}
}
```

See the [library guide](docs/library.md) for `portspec`, `inspect`, `freeport`, `killer`, `scan` and `waitfor`.

## Responsible use

`portcheck scan` makes ordinary TCP connections and has no stealth features. Only
scan hosts and networks you own or are authorized to test.

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for setup and
standards (every change is tested on Linux, macOS and Windows, with a 100% coverage
gate on the library), and [TASKS.md](TASKS.md) for the roadmap and status.
Please follow the [Code of Conduct](CODE_OF_CONDUCT.md), and report vulnerabilities
privately as described in [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © 2026 Royston D and portcheck contributors
