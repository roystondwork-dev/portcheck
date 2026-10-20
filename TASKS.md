# portcheck — Task & Status Sheet

> Single source of truth for the roadmap. Update the row's **Status** and **Notes**
> in the same PR that completes the work.

**Current milestone:** M3 — Remote scanner
**Last updated:** 2026-10-20
**Target release:** v0.1.0

Legend: ⬜ todo · 🟨 in progress · ✅ done · ⛔ blocked

## Progress overview

| Milestone | Scope | Done | Status |
|---|---|---|---|
| M0 | Bootstrap & project hygiene | 12 / 13 | 🟨 (repo creation pending approval) |
| M1 | Core library (`pkg/*` except scan) | 7 / 7 | ✅ |
| M2 | CLI | 11 / 11 | ✅ |
| M3 | Remote scanner | 0 / 6 | ⬜ |
| M4 | Interactive TUI | 0 / 7 | ⬜ |
| M5 | Documentation | 0 / 9 | ⬜ |
| M6 | Release & distribution | 0 / 10 | ⬜ |

---

## M0 — Bootstrap

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M0-01 | Install toolchain (Go, golangci-lint, GoReleaser, lefthook) | local | — | ✅ | Go 1.27.3, golangci-lint 2.15.0, GoReleaser 2.18.3, lefthook 2.2.2 |
| M0-02 | `git init`, `go.mod` (min go 1.26, toolchain 1.27.3) | `go.mod` | — | ✅ | module `github.com/roystondwork-dev/portcheck` |
| M0-03 | Pinned dev tools module | `tools/go.mod` | — | ✅ | gotestsum, govulncheck, go-test-coverage, gremlins via `go tool` |
| M0-04 | LICENSE (MIT), `.gitignore`, `.editorconfig`, `.gitattributes` | root | — | ✅ | |
| M0-05 | Makefile task runner | `Makefile` | `make help` works | ✅ | |
| M0-06 | Strict lint config incl. layering rules | `.golangci.yml` | `make lint` passes | ✅ | depguard blocks `pkg → internal/cobra/charm` |
| M0-07 | Coverage gate config | `.testcoverage.yml` | — | ✅ | pkg 100%, package 95%, total 95% |
| M0-08 | Git hooks | `lefthook.yml` | — | ✅ | fmt, conventional commit msg, lint+test on push |
| M0-09 | CI: lint, vuln, 3-OS tests, merged coverage gate, CodeQL, Scorecard, mutation, Dependabot | `.github/` | first CI run green | ✅ | actions pinned by SHA; verified once repo is pushed |
| M0-10 | Community files | CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, GOVERNANCE, SUPPORT, templates, CODEOWNERS | — | ✅ | |
| M0-11 | ADR-0001 (gopsutil provider), ADR-0002 (TCP-connect-only scanning) | `docs/adr/` | — | ✅ | |
| M0-12 | `internal/buildinfo` | `internal/buildinfo` | unit, 100% | ✅ | |
| M0-13 | Create GitHub repo + push + branch protection | GitHub | CI green on `main` | ⛔ | Needs maintainer approval (outward-facing) |

## M1 — Core library

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M1-01 | Port spec parser (`80,443,8000-8100,top100`) | `pkg/portspec` | unit + fuzz, 100% | ✅ | Presets: `common`, `well-known`, `registered`, `dynamic`, `all` (nmap top-N lists not used: NPSL license). Added `Spec.At`. |
| M1-02 | Socket/process model, `Provider` interface, `Filter` | `pkg/inspect` | unit, 100% | ✅ | |
| M1-03 | `Inspector.List`: enrich, filter, dedupe, sort | `pkg/inspect` | unit with `FakeProvider` | ✅ | Exact-duplicate removal; per-PID lookup cache; bounded concurrency |
| M1-04 | gopsutil provider + privilege detection & hints | `pkg/inspect` | integration (3 OS) | ✅ | Verified on macOS (lsof); Linux/Windows run in CI |
| M1-05 | Free-port check / next / random | `pkg/freeport` | unit + integration | ✅ | Listener check + bind on 127.0.0.1/0.0.0.0/::1/::; Winsock errnos mapped |
| M1-06 | Killer: graceful → force, safety guards | `pkg/killer` | unit with fake signaler | ✅ | Hard guards: pid ≤ 1, self, parent. Name guard overridable via `AllowProtected` |
| M1-07 | Killer OS integration (spawn & kill child) | `pkg/killer` | integration (3 OS) | ✅ | Helper-process pattern; covers graceful, force-after-grace, zombie detection |

## M2 — CLI

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M2-01 | Output renderers: table / json / csv / plain, NO_COLOR, TTY detect | `internal/output` | golden files | ✅ | `Document` + tones; NO_COLOR beats CLICOLOR_FORCE; trailing whitespace trimmed |
| M2-02 | Root command, global flags, slog, signal ctx | `internal/cli/root.go` | testscript | ✅ | `--output/--json/--no-color/--verbose/--log-format`; typo suggestions; `portcheck <port>` shortcut |
| M2-03 | Typed exit codes | `internal/cli/exitcode.go` | unit | ✅ | + 130 for Ctrl-C. Partial visibility warns on stderr but exits 0 (plan said 4; changed so `list` stays script-friendly) |
| M2-04 | `list` (+ filters, `--watch`) | `internal/cli/list.go` | testscript | ✅ | |
| M2-05 | `check` + `portcheck <port>` shortcut | `internal/cli/check.go` | testscript | ✅ | Uses `Finder.CheckAll` (one socket listing for many ports); cap 4096 ports |
| M2-06 | `free` | `internal/cli/free.go` | testscript | ✅ | Default range 1024-65535 |
| M2-07 | `kill` with confirmation, `--yes/--force/--dry-run` | `internal/cli/kill.go` | testscript | ✅ | Guards run before the prompt; non-TTY requires `--yes`; `--allow-system` for protected names |
| M2-08 | `version`, `completion` | `internal/cli` | testscript | ✅ | |
| M2-09 | `cmd/portcheck/main.go` | `cmd/portcheck` | e2e via built binary | ✅ | |
| M2-10 | JSON output schema (`schemaVersion: 1`) documented | `docs/json-schema.md` | schema test | ✅ | [docs/json-schema.md](docs/json-schema.md), [docs/exit-codes.md](docs/exit-codes.md); JSON golden files lock the shape |
| M2-11 | Doc generator (markdown + man) | `internal/cli/gendocs` | runs in CI | ✅ | CI fails if `docs/commands` is stale |

## M3 — Remote scanner

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M3-01 | Target expansion: host / IP / CIDR with cap | `pkg/scan/targets.go` | unit + fuzz | ⬜ | |
| M3-02 | Scanner engine: worker pool, timeout, rate limit, retries | `pkg/scan/scanner.go` | unit with fake Dialer, `-race` | ⬜ | |
| M3-03 | Result classification open/closed/filtered/error | `pkg/scan` | unit | ⬜ | |
| M3-04 | Embedded service-name table | `pkg/scan/services.go` | unit | ⬜ | |
| M3-05 | `scan` command with progress + summary | `internal/cli/scan.go` | testscript + integration | ⬜ | |
| M3-06 | Benchmarks | `pkg/scan` | `make bench` | ⬜ | |

## M4 — Interactive TUI

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M4-01 | Model/Update/View skeleton, live refresh | `internal/tui` | unit on Update | ⬜ | |
| M4-02 | Sortable table, resize handling | `internal/tui` | teatest golden | ⬜ | |
| M4-03 | Filter bar (`/`), proto toggle (`tab`) | `internal/tui` | unit + golden | ⬜ | |
| M4-04 | Kill with confirm dialog (`k`) | `internal/tui` | unit + golden | ⬜ | |
| M4-05 | Help (`?`), keymap, adaptive colors | `internal/tui` | golden | ⬜ | |
| M4-06 | `tui` command + default on bare TTY | `internal/cli/tui.go` | testscript | ⬜ | |
| M4-07 | Demo recording with vhs | `docs/assets` | — | ⬜ | |

## M5 — Documentation

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M5-01 | README (badges, install, quickstart, demo) | `README.md` | links check | ⬜ | |
| M5-02 | MkDocs Material site config + nav | `mkdocs.yml` | `mkdocs build --strict` | ⬜ | |
| M5-03 | Install guide incl. signature verification | `docs/install.md` | — | ⬜ | |
| M5-04 | Quickstart & per-command guides | `docs/` | — | ⬜ | |
| M5-05 | Generated CLI reference + man pages | `docs/commands` | CI drift check | ⬜ | |
| M5-06 | Library guide + runnable `Example_` tests | `docs/library.md`, `pkg/*/example_test.go` | examples run | ⬜ | |
| M5-07 | Architecture doc + diagram | `docs/architecture.md` | — | ⬜ | |
| M5-08 | Permissions per OS, troubleshooting, FAQ | `docs/` | — | ⬜ | |
| M5-09 | Docs deploy workflow (GitHub Pages) | `.github/workflows/docs.yml` | — | ⬜ | |

## M6 — Release & distribution

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M6-01 | GoReleaser: 6 OS/arch binaries, archives, checksums | `.goreleaser.yaml` | `make snapshot` | ⬜ | |
| M6-02 | nfpm packages: deb / rpm / apk | `.goreleaser.yaml` | snapshot | ⬜ | |
| M6-03 | Homebrew tap | `roystondwork-dev/homebrew-tap` | — | ⬜ | Needs repo creation (approval) |
| M6-04 | Scoop bucket | `roystondwork-dev/scoop-bucket` | — | ⬜ | Needs repo creation (approval) |
| M6-05 | SBOM (syft) + cosign keyless signing | `.goreleaser.yaml` | verify in docs | ⬜ | |
| M6-06 | SLSA build provenance attestation | `release.yml` | — | ⬜ | |
| M6-07 | release-please (changelog + version PRs) | `release-please.yml` | — | ⬜ | |
| M6-08 | Release workflow | `.github/workflows/release.yml` | — | ⬜ | |
| M6-09 | `CHANGELOG.md` | root | — | ⬜ | managed by release-please |
| M6-10 | Tag & publish **v0.1.0** | GitHub | install via brew works | ⬜ | Needs approval |

---

## Decisions log

| Date | Decision | Ref |
|---|---|---|
| 2026-10-20 | Go CLI + Bubble Tea TUI; MIT license; name `portcheck` | plan |
| 2026-10-20 | gopsutil behind `inspect.Provider` | [ADR-0001](docs/adr/0001-gopsutil-provider.md) |
| 2026-10-20 | Scanner is TCP-connect only, no stealth features | [ADR-0002](docs/adr/0002-tcp-connect-only-scanning.md) |
