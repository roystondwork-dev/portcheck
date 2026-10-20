# portcheck — Task & Status Sheet

> Single source of truth for the roadmap. Update the row's **Status** and **Notes**
> in the same PR that completes the work.

**Current milestone:** ✅ v0.1.0 released, all milestones complete
**Last updated:** 2026-10-20
**Target release:** v0.1.0

Legend: ⬜ todo · 🟨 in progress · ✅ done · ⛔ blocked

## Progress overview

| Milestone | Scope | Done | Status |
|---|---|---|---|
| M0 | Bootstrap & project hygiene | 13 / 13 | ✅ |
| M1 | Core library (`pkg/*` except scan) | 7 / 7 | ✅ |
| M2 | CLI | 11 / 11 | ✅ |
| M3 | Remote scanner | 6 / 6 | ✅ |
| M4 | Interactive TUI | 7 / 7 | ✅ |
| M5 | Documentation | 9 / 9 | ✅ |
| M6 | Release & distribution | 10 / 10 | ✅ |

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
| M0-09 | CI: lint, vuln, 3-OS tests, merged coverage gate, CodeQL, Scorecard, mutation, Dependabot | `.github/` | first CI run green | ✅ | Actions pinned by SHA; green on Linux, macOS, Windows |
| M0-10 | Community files | CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, GOVERNANCE, SUPPORT, templates, CODEOWNERS | — | ✅ | |
| M0-11 | ADR-0001 (gopsutil provider), ADR-0002 (TCP-connect-only scanning) | `docs/adr/` | — | ✅ | |
| M0-12 | `internal/buildinfo` | `internal/buildinfo` | unit, 100% | ✅ | |
| M0-13 | Create GitHub repo + push + branch protection | GitHub | CI green on `main` | ✅ | [roystondwork-dev/portcheck](https://github.com/roystondwork-dev/portcheck); first CI run caught a Linux/Windows bug (remote `0.0.0.0:0`), fixed; `main` requires all CI checks + linear history |

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
| M3-01 | Target expansion: host / IP / CIDR with cap | `pkg/scan/targets.go` | unit + fuzz | ✅ | Hostnames prefer IPv4; de-duplicated; huge prefixes rejected before enumeration; default cap 1024 |
| M3-02 | Scanner engine: worker pool, timeout, rate limit, retries | `pkg/scan/scanner.go` | unit with fake Dialer, `-race` | ✅ | Defaults: 100 workers, 1s timeout, 500 conn/s; retries apply to timeouts only; stress-tested 30× under `-race` |
| M3-03 | Result classification open/closed/filtered/error | `pkg/scan` | unit | ✅ | Per-OS "connection refused" (Winsock 10061 on Windows) |
| M3-04 | Embedded service-name table | `pkg/scan/services.go` | unit | ✅ | ~60 well-known names; labels only, no fingerprinting |
| M3-05 | `scan` command with progress + summary | `internal/cli/scan.go` | testscript + integration | ✅ | Live progress on TTY, summary line, `--open`, authorization note for public addresses, partial results on Ctrl-C |
| M3-06 | Benchmarks | `pkg/scan` | `make bench` | ✅ | ~4k probes in 3.2 ms with a fake dialer (scheduling overhead is negligible) |

## M4 — Interactive TUI

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M4-01 | Model/Update/View skeleton, live refresh | `internal/tui` | unit on Update | ✅ | Pure model; list/kill run as `tea.Cmd`s via interfaces; cursor follows the selected socket across refreshes |
| M4-02 | Sortable table, resize handling | `internal/tui` | teatest golden | ✅ | Custom table renderer (deterministic, width-aware truncation). Golden snapshots of ANSI-stripped screens instead of teatest (no tagged v2 release) |
| M4-03 | Filter bar (`/`), proto toggle (`tab`) | `internal/tui` | unit + golden | ✅ | Live filter over port/proto/address/state/pid/process/user |
| M4-04 | Kill with confirm dialog (`k`) | `internal/tui` | unit + golden | ✅ | `x`/`k`/`delete` graceful, `K` force; verified killing a real server from the TUI over a PTY |
| M4-05 | Help (`?`), keymap, adaptive colors | `internal/tui` | golden | ✅ | Hidden-owner rows dimmed, listeners green; `c` copies PID via OSC 52 |
| M4-06 | `tui` command + default on bare TTY | `internal/cli/tui.go` | testscript | ✅ | Bare `portcheck` opens the TUI only when stdin and stdout are terminals; otherwise prints help |
| M4-07 | Demo recording with vhs | `docs/assets` | — | ✅ | `docs/assets/demo.gif` rendered with vhs (`make demo`); frames reviewed |

## M5 — Documentation

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M5-01 | README (badges, install, quickstart, demo) | `README.md` | links check | ✅ | Badges, demo GIF, features, install, quickstart, library snippet, responsible use |
| M5-02 | MkDocs Material site config + nav | `mkdocs.yml` | `mkdocs build --strict` | ✅ | MkDocs Material 9.7 pinned (`docs/requirements.txt`); strict link + anchor validation |
| M5-03 | Install guide incl. signature verification | `docs/install.md` | — | ✅ | Homebrew/Scoop/packages/Go; cosign bundle + checksum + `gh attestation` verification (finalized in M6) |
| M5-04 | Quickstart & per-command guides | `docs/` | — | ✅ | Quickstart + guides for list/check/free/kill/scan/tui + scripting & CI recipes; port spec reference |
| M5-05 | Generated CLI reference + man pages | `docs/commands` | CI drift check | ✅ | Generated in CI before every docs build |
| M5-06 | Library guide + runnable `Example_` tests | `docs/library.md`, `pkg/*/example_test.go` | examples run | ✅ | `Example` tests in every `pkg/*`; one caught a wrong port count in the draft docs |
| M5-07 | Architecture doc + diagram | `docs/architecture.md` | — | ✅ | Mermaid layer diagram + `kill` sequence diagram; design principles |
| M5-08 | Permissions per OS, troubleshooting, FAQ | `docs/` | — | ✅ | Per-OS permission matrix (lsof / procfs / IP Helper), Windows excluded port ranges, FAQ |
| M5-09 | Docs deploy workflow (GitHub Pages) | `.github/workflows/docs.yml` | — | ✅ | `docs.yml` builds on PRs, deploys `main` to https://roystondwork-dev.github.io/portcheck/ |

## M6 — Release & distribution

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M6-01 | GoReleaser: 6 OS/arch binaries, archives, checksums | `.goreleaser.yaml` | `make snapshot` | ✅ | darwin/linux/windows × amd64/arm64; tar.gz (zip on Windows) with completions + man pages; reproducible (`-trimpath`, commit timestamps) |
| M6-02 | nfpm packages: deb / rpm / apk | `.goreleaser.yaml` | snapshot | ✅ | deb/rpm/apk with binary, bash/zsh/fish completions, man pages, license; contents verified |
| M6-03 | Homebrew tap | `roystondwork-dev/homebrew-tap` | — | ✅ | [homebrew-tap](https://github.com/roystondwork-dev/homebrew-tap) (cask, macOS + Linux); `brew install roystondwork-dev/tap/portcheck` verified on macOS |
| M6-04 | Scoop bucket | `roystondwork-dev/scoop-bucket` | — | ✅ | [scoop-bucket](https://github.com/roystondwork-dev/scoop-bucket) updated by the v0.1.0 release |
| M6-05 | SBOM (syft) + cosign keyless signing | `.goreleaser.yaml` | verify in docs | ✅ | SPDX SBOM per archive (syft, verified locally); keyless cosign bundle for `checksums.txt`; identity documented |
| M6-06 | SLSA build provenance attestation | `release.yml` | — | ✅ | `actions/attest-build-provenance` over all archives, packages and checksums |
| M6-07 | release-please (changelog + version PRs) | `release.yml` | — | ✅ | Merged into `release.yml`; minor bumps pre-1.0; `RELEASE_PLEASE_TOKEN` recommended so CI runs on release PRs |
| M6-08 | Release workflow | `.github/workflows/release.yml` | — | ✅ | `release.yml` (release-please → GoReleaser → provenance); manual re-run for an existing tag; CI runs `goreleaser check` + full snapshot; actionlint added |
| M6-09 | `CHANGELOG.md` | root | — | ✅ | Seeded; release-please fills it from Conventional Commits |
| M6-10 | Tag & publish **v0.1.0** | GitHub | install via brew works | ✅ | [v0.1.0](https://github.com/roystondwork-dev/portcheck/releases/tag/v0.1.0): 20 assets; cosign bundle, checksums and provenance verified; tampered/unattested files rejected; brew install verified |

---

## Decisions log

| Date | Decision | Ref |
|---|---|---|
| 2026-10-20 | Go CLI + Bubble Tea TUI; MIT license; name `portcheck` | plan |
| 2026-10-20 | gopsutil behind `inspect.Provider` | [ADR-0001](docs/adr/0001-gopsutil-provider.md) |
| 2026-10-20 | Scanner is TCP-connect only, no stealth features | [ADR-0002](docs/adr/0002-tcp-connect-only-scanning.md) |
| 2026-10-20 | First release is 0.1.0 (`initial-version`), not release-please's default 1.0.0 | #6 |
| 2026-10-20 | TUI uses a custom table + golden screen snapshots instead of bubbles/table + teatest | M4 notes |
