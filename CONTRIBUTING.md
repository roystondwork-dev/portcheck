# Contributing to portcheck

Thanks for your interest in improving portcheck! This guide explains how to set
up a development environment, the standards every change must meet, and how
changes get released.

By participating you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## Ways to contribute

- **Report bugs** or **request features** via [GitHub Issues](https://github.com/roystondwork-dev/portcheck/issues/new/choose).
- **Improve docs** — typos, examples, clarifications are always welcome.
- **Write code** — look for issues labelled `good first issue` or `help wanted`,
  and check [TASKS.md](TASKS.md) for the current roadmap.
- **Report security issues privately** — see [SECURITY.md](SECURITY.md). Never open a public issue for a vulnerability.

## Development setup

Requirements:

| Tool | Version | Install (macOS) |
|---|---|---|
| Go | see `go.mod` (`toolchain`) | `brew install go` |
| golangci-lint | v2.x | `brew install golangci-lint` |
| lefthook (optional, git hooks) | v2.x | `brew install lefthook` |
| GoReleaser (optional, release builds) | v2.x | `brew install goreleaser` |

Everything else (gotestsum, govulncheck, coverage gate, mutation testing) is
pinned in `tools/go.mod` and run through `go tool`, so no extra installs are needed.

```sh
git clone https://github.com/roystondwork-dev/portcheck.git
cd portcheck
lefthook install      # optional: format on commit, lint + test on push
make help             # list all tasks
make build            # -> ./bin/portcheck
make check            # lint + actionlint + vuln + tests + coverage gate
make docs-cli         # regenerate docs/commands (CI fails if it is stale)
```

## Project layout

```
cmd/portcheck       entry point (thin)
internal/cli        cobra commands — argument parsing and wiring only
internal/cli/gendocs  CLI reference + man page generator
internal/output     table/json/csv/plain renderers
internal/tui        Bubble Tea interactive UI
internal/buildinfo  version/commit/date of the binary
internal/project    .portcheck.yaml loading and validation
internal/testutil   shared test helpers (golden files)
pkg/portspec        port list/range parsing          (public library)
pkg/inspect         local socket → process lookup    (public library)
pkg/freeport        free-port detection              (public library)
pkg/killer          safe process termination         (public library)
pkg/scan            TCP connect scanner              (public library)
pkg/waitfor         poll until a port opens          (public library)
pkg/containers      Docker/Podman port attribution   (public library)
pkg/wsl             WSL distro port attribution      (public library)
```

Dependencies point one way: `cmd → internal/cli → internal/* → pkg/*`. `depguard`
enforces the most important part: packages under `pkg/` must never import
`internal/`, cobra or Charm libraries. The rest is a reviewed convention.
See [docs/architecture.md](docs/architecture.md) for details.

## Standards every change must meet

1. **Tests for every line.** `pkg/*` is held to **100% line coverage**; everything
   else to ≥ 95% per package and in total, and ≥ 90% per file. CI enforces this
   with `.testcoverage.yml`. Prefer table-driven tests
   with `t.Parallel()`, use fakes (not mocks frameworks) at OS boundaries, and add
   a fuzz target for any parser.
2. **Integration tests** that touch real sockets or processes live behind the
   `integration` build tag and run on Linux, macOS and Windows in CI.
3. **Lint clean.** `make lint` must pass. `//nolint` requires a specific linter and an explanation.
4. **Documented.** Every exported identifier has a doc comment. User-facing
   changes update `docs/` and, for commands, the CLI help text.
5. **Cross-platform.** OS-specific code lives in `_windows.go` / `_unix.go` files
   (or `_darwin.go` / `_linux.go` when truly needed) behind a small interface.

## VS Code extension

The extension lives in `extensions/vscode/` (TypeScript, esbuild, Vitest, `@vscode/test-electron`):

```sh
make build                          # the integration tests use bin/portcheck
cd extensions/vscode && npm ci
npm run check                       # typecheck, lint, unit tests (100% coverage gate), bundle
npm run test:integration            # runs the extension inside a downloaded VS Code
```

It must stay a thin client of `portcheck --json` (see ADR-0004): port logic belongs in Go.

## Browser extension

The extension lives in `extensions/browser/` (Manifest V3, TypeScript, esbuild, Vitest,
Playwright):

```sh
make build                          # the end-to-end tests use bin/portcheck
cd extensions/browser && npm ci
npm run check                       # typecheck, lint, unit tests (100% coverage gate), bundle
npx playwright install chromium     # once
npx playwright test                 # the extension in a real Chromium with the real portcheck
```

Like the VS Code extension, it's a thin client: port logic and safety checks belong in
Go (see ADR-0005).

## Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/) — they drive
the changelog and semantic version bumps via release-please.

```
feat(scan): support CIDR targets
fix(inspect): dedupe dual-stack listeners on Linux
docs: explain macOS permission limits
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.
Breaking changes: add `!` after the type (`feat!:`) and a `BREAKING CHANGE:` footer.

## Pull request process

1. Fork, create a branch from `main` (`feat/short-description`).
2. Make your change with tests and docs; run `make check`.
3. Open a PR using the template. Keep PRs focused — one logical change each.
4. A maintainer reviews; CI must be green. We squash-merge using the PR title,
   so it must be a valid Conventional Commit.

## Updating golden files

Renderer and TUI snapshot tests compare against files in `testdata/`. After an
intentional output change:

```sh
go test ./internal/output/... ./internal/tui/... -update
```

Review the resulting diff carefully before committing.

## Releases

Maintainers merge the release-please PR; this tags `vX.Y.Z` and GoReleaser
publishes signed binaries, packages, SBOMs and provenance. See [docs/releasing.md](docs/releasing.md).
