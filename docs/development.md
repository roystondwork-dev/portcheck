# Development & testing

For setup, commit conventions and the PR process, see
[CONTRIBUTING.md](https://github.com/roystondwork-dev/portcheck/blob/main/CONTRIBUTING.md).
This page explains how portcheck is tested and why.

## Commands

| Command | What it does |
|---|---|
| `make test` | unit tests with `-race` and shuffled order |
| `make test-integration` | adds `-tags integration`: real sockets, processes and DNS |
| `make cover-check` | integration tests + coverage gate (`.testcoverage.yml`) |
| `make check` | lint + govulncheck + cover-check, exactly what CI runs |
| `make fuzz FUZZTIME=1m` | every fuzz target for the given time |
| `make mutation` | mutation testing of `pkg/` with gremlins |
| `make bench` | benchmarks |
| `make docs` | regenerate the CLI reference and build this site with `--strict` |
| `make demo` | re-record `docs/assets/demo.gif` with vhs |

## Test layers

| Layer | Technique | Where |
|---|---|---|
| Pure logic | table-driven tests, `t.Parallel()`, go-cmp diffs | everywhere |
| Parsers | native Go fuzzing (`FuzzParse`, `FuzzExpand`) | `pkg/portspec`, `pkg/scan` |
| OS boundaries | hand-written fakes for `Provider`, `ops`, `Dialer`, `Resolver`, `bind` | `pkg/*` |
| Real OS behavior | `//go:build integration`: real listeners, child processes killed for real, loopback scans | `pkg/*/integration_test.go` |
| Commands | in-process tests with a fake `App`; every flag, error path and exit code | `internal/cli/*_test.go` |
| Real binary | [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript) scenarios against the compiled CLI and a helper process holding a port | `internal/cli/testdata/script/*.txtar` |
| Rendering | golden files for every format and TUI screen | `internal/*/testdata/*.golden` |
| Test quality | mutation testing (weekly in CI) | `.github/workflows/mutation.yml` |

## Coverage policy

CI runs the full suite, integration tests included, on **Linux, macOS and Windows**,
merges the three coverage profiles, and enforces:

- **100%** line coverage for `pkg/*` (the public library);
- ≥ 95% per package and in total, and ≥ 90% per file, elsewhere.

OS-specific files (`*_windows.go`, `*_unix.go`) are only measured on their own OS,
which is why profiles are merged before the gate. The only exclusions are
`cmd/portcheck/main.go` (one line calling `cli.Main`, which is tested directly) and
test-only helpers; they are listed explicitly in `.testcoverage.yml`.

Coverage shows that every line ran, not that its result was checked. Mutation testing
(`make mutation`) covers that gap: it changes the code and verifies that a test fails.

## Golden files

After an intentional change to output or the TUI:

```sh
go test ./internal/output/... ./internal/tui/... -update
git diff internal/*/testdata   # review every change
```

## Fakes, not mocks

Tests use small hand-written fakes (`inspect.FakeProvider`, `fakeDialer`, `fakeOps`, …)
that model behavior: a fake process that ignores SIGTERM, a dialer that times out,
and so on. They don't record expected call sequences, so tests stay readable and don't
break on refactors.
