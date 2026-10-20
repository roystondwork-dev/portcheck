# portcheck — Task & Status Sheet

> Single source of truth for the roadmap. Update the row's **Status** and **Notes**
> in the same PR that completes the work.

**Current milestone:** M17 — browser extension (Chrome, Edge). Open: restore the macOS jobs and the coverage gate as required checks on `main` (made optional on 2026-10-20 while GitHub had no macOS runner capacity), manual WSL check (M11-04, deferred by maintainer), winget review (M7-06), Open VSX namespace claim (October 2027)
**Last updated:** 2026-10-20
**Latest release:** [v0.5.0](https://github.com/roystondwork-dev/portcheck/releases/tag/v0.5.0)

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
| M7 | Post-release hardening & distribution (v0.1.1 → v0.2.0) | 8 / 9 | 🟨 (winget awaiting Microsoft review) |
| M8 | Docker attribution | 7 / 7 | ✅ |
| M9 | `portcheck run` | 6 / 6 | ✅ (released in v0.3.0) |
| M10 | Parent process ("started by") | 5 / 5 | ✅ |
| M11 | WSL distro attribution | 3 / 4 | 🟨 (needs a test on a Windows machine with WSL) |
| M12 | `.portcheck.yaml` + `portcheck doctor` | 5 / 5 | ✅ |
| M13 | Native macOS provider (no `lsof`) | 4 / 4 | ✅ (released in v0.4.0) |
| M14 | VS Code extension — MVP (ports sidebar, commands) | 7 / 7 | ✅ |
| M15 | VS Code extension — USP (terminal "port in use" → one-click fix; project ports) | 5 / 5 | ✅ |
| M16 | VS Code extension — packaging & publishing (VSIX, Marketplace, Open VSX) | 4 / 4 | ✅ |
| M17 | Browser extension (Chrome, Edge; native messaging) | 7 / 8 | 🟨 |

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

## M7 — Post-release hardening & distribution

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M7-01 | Fix initial version (release-please defaulted to 1.0.0) | `release-please-config.json` | — | ✅ | `initial-version: 0.1.0` (#6) |
| M7-02 | Correct macOS low-port binding docs; tidy changelog | `docs/` | strict docs build | ✅ | Verified: macOS allows wildcard-only unprivileged low-port binds (#7); released as v0.1.1 |
| M7-03 | Codecov upload, config and badges | `codecov.yml`, `README.md` | upload verified | ✅ | Targets mirror `.testcoverage.yml` (#9) |
| M7-04 | Enforce branch protection for admins | GitHub settings | — | ✅ | All changes via PR with 6 required checks |
| M7-05 | winget publishing | `.goreleaser.yaml`, `release.yml` | snapshot manifest | ✅ | Package ID `roystondwork-dev.portcheck`; fork `roystondwork-dev/winget-pkgs` (#10) |
| M7-06 | First winget submission accepted | microsoft/winget-pkgs#456772 (0.5.0) | Microsoft validation | 🟨 | Older submissions (0.1.1–0.4.0) closed as superseded. 0.5.0 validated; needs the maintainer's CLA comment, then a moderator |
| M7-09 | Release **v0.2.0** (`wait`, container attribution, kill safety fix, docs audit) | GitHub | signatures + brew install | ✅ | [v0.2.0](https://github.com/roystondwork-dev/portcheck/releases/tag/v0.2.0): 20 assets; cosign, checksums, provenance verified; Homebrew upgrade verified against a real container |
| M7-07 | `portcheck wait` + `pkg/waitfor` | `internal/cli/wait.go`, `pkg/waitfor` | unit + testscript, 100% | ✅ | Open or `--free`; parallel targets; replaces scan loops in CI docs (#11) |
| M7-08 | Independent accuracy audit of README, docs site and contributor docs against code and binary | `docs/`, `README.md`, `CONTRIBUTING.md`, `SECURITY.md` | strict docs build | ✅ | 27 findings fixed (5 factual errors incl. package filenames, check output order, unsafe README snippet, signing scope); `--no-color` now reaches the TUI (#13) |

## M8 — Docker attribution

Goal: ports published by containers show the **container** (name, image), not the
proxy process holding them (`docker-proxy`, `com.docker.backend`, …), and `kill`
never stops Docker's infrastructure by accident.

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M8-01 | Docker Engine API client: unix socket / named pipe / tcp, `DOCKER_HOST`, per-OS defaults; Podman-compatible | `pkg/containers` | unit with fake API server, 100% | ✅ | Docker Desktop, Colima, OrbStack, rootless, Podman; `DOCKER_HOST`; go-winio for pipes; TLS/ssh unsupported (documented) |
| M8-02 | Enrichment hook in `inspect` + `Socket.Container` | `pkg/inspect` | unit | ✅ | 500 ms budget, errors ignored; `--name` matches container names; ~10 ms measured on macOS |
| M8-03 | Show containers in `list`/`check` tables, JSON and TUI | `internal/output`, `internal/tui` | golden | ✅ | `docker:web` label; JSON `container` {id, name, image, runtime} |
| M8-04 | `kill` and TUI refuse container ports with a `docker stop` hint | `internal/cli/kill.go`, `internal/tui` | unit + golden | ✅ | Previously `kill` on a published port would stop Docker Desktop's backend |
| M8-05 | Protect Docker/WSL infrastructure processes in `killer` | `pkg/killer` | unit | ✅ | Docker, containerd, rootlesskit, slirp4netns, vpnkit, WSL relay/service, vmmem |
| M8-06 | Integration test against a real Docker daemon | CI (Linux) | integration | ✅ | Real busybox container on a random port; passes on Docker Desktop (macOS) and runs on GitHub's Ubuntu runner |
| M8-07 | Docs: guide section, permissions/troubleshooting, JSON schema | `docs/` | strict docs build | ✅ | New Containers guide incl. limitations (no-userland-proxy, socket permissions, WSL) |

## M9 — `portcheck run`

Goal: start a dev command on a usable port in one step: use `--port` if free,
otherwise explain, pick the next free port (`--next`) or stop the holder (`--kill`),
then run the command with `$PORT` set and its exit code passed through.

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M9-01 | Port selection: `--port`, `--next`, `--kill` (reusing kill's guards), `--range` default 3000-9999 | `internal/cli/run.go` | unit with fakes | ✅ | Busy without flags explains holder (process, container or pid) and exits 1; `--kill` waits up to 10s for the port to free |
| M9-02 | Child process runner: inherit stdio, `$PORT`/`--env`, `{port}` substitution, exit-code passthrough (incl. 126/127, 128+signal) | `internal/cli/runner*.go` | unit with a real child process | ✅ | Test binary doubles as the child (exit codes, env, args, SIGTERM death = 143, 126/127) |
| M9-03 | Signal forwarding (SIGTERM/SIGHUP; Ctrl+C reaches the child via the terminal) | `internal/cli/runner_unix.go` | unit | ✅ | Ctrl+C is deliberately not relayed (the child already gets it from the terminal) |
| M9-04 | `--wait`: report when the command accepts connections | `internal/cli/run.go` | unit | ✅ | Verified on macOS: "ready on localhost:8791 (253ms)" for a real HTTP server |
| M9-05 | End-to-end scenarios against the built binary | `testdata/script/run.txtar` | testscript (3 OS) | ✅ | Auto port, busy, `--next`, `--kill`, exit-code passthrough, not found |
| M9-06 | Docs: guide, scripting recipes, exit codes, README | `docs/` | strict docs build | ✅ | New `run` guide; quickstart, scripting, exit codes, README, home page |

## M10 — Parent process

Goal: show who started a process (`node ← npm ← zsh`) where it helps decide whether to stop it.

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M10-01 | `ProcessInfo.Ancestors` (nearest first, ≤ 3, stops at init/launchd) from the system provider | `pkg/inspect` | unit with fake handles, integration | ✅ | Verified on macOS: Python ← bash ← zsh; reparented processes correctly show none; no measurable overhead |
| M10-02 | `kill` confirmation shows "started by …" | `internal/cli/kill.go` | unit | ✅ | `PID 99087 Python (port 8794), started by bash (99085) ← zsh (99084) ← …` |
| M10-03 | TUI: selected row's ancestry + command in the status line; in the kill confirmation | `internal/tui` | golden | ✅ | Hidden-owner rows still show the privilege hint |
| M10-04 | `list --wide` (PARENT, COMMAND) and JSON `process.ancestors` | `internal/output`, `internal/cli/list.go` | golden | ✅ | `-w/--wide`; command truncated to 60 runes in tables |
| M10-05 | Docs | `docs/` | strict docs build | ✅ | list, kill, TUI, JSON schema, library |

## M11 — WSL distro attribution

Goal: on Windows, ports forwarded from WSL show the distro and the Linux process
(`wsl:Ubuntu/node`) instead of `wslrelay.exe`, and `kill` explains how to stop it inside WSL.

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M11-01 | Generic groundwork: `Container.Process`/`ProcessPID`, per-enricher time budget, runtime-specific stop hints | `pkg/inspect`, `internal/output`, `internal/cli`, `internal/tui` | unit + golden | ✅ | Hints now come from `output.DescribeContainer`/`StopHint`; enrichers can request a longer budget (`EnrichTimeouter`) |
| M11-02 | `pkg/wsl` enricher: running distros (UTF-8/UTF-16), per-distro listeners (`ss -Hltnp`, `/proc/net/tcp` fallback), matching | `pkg/wsl` | unit with captured formats, 100% | ✅ | UTF-16LE (with/without BOM) and UTF-8 output; `ss` + `/proc/net/tcp{,6}`; failing distros skipped; 3s budget |
| M11-03 | Enable on Windows | `internal/cli/app.go` | unit | ✅ | Windows only; zero cost without relay sockets |
| M11-04 | Docs + manual verification on a Windows machine with WSL | `docs/` | strict docs build; maintainer test | 🟨 | Docs done ("Containers & WSL"). Manual check on a Windows machine with a running distro pending |

## M12 — `.portcheck.yaml` + `portcheck doctor`

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M12-01 | Config file: find upward, strict YAML (unknown keys rejected with line numbers), validation, defaults | `internal/project` | unit, 100% | ✅ | `go.yaml.in/yaml/v3` (maintained successor of gopkg.in/yaml.v3) |
| M12-02 | Published JSON Schema for editors, kept in sync with the struct by a test | `docs/schema/portcheck.schema.json` | unit | ✅ | Served at the docs site URL in `$id` |
| M12-03 | `portcheck doctor`: parallel checks, holder-specific fixes (kill / docker / wsl / sudo), exit 0/1/2 | `internal/cli/doctor.go`, `internal/output` | unit + golden + testscript | ✅ | Verified on macOS against real processes |
| M12-04 | `run --port <name>` | `internal/cli/run.go` | unit + testscript | ✅ | Unknown names list the known ones |
| M12-05 | Docs | `docs/guide/doctor.md` + schema, exit codes, run, README | strict docs build | ✅ | |

## M13 — Native macOS provider

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M13-01 | libproc socket enumeration (portable logic + `socket_fdinfo` decoder) | `pkg/inspect/libproc.go` | unit on every OS, 100% | ✅ | Offsets from the macOS SDK; identical on arm64 and x86_64 |
| M13-02 | purego binding to libSystem (no cgo), `lsof` fallback on failure | `pkg/inspect/libproc_darwin.go`, `system.go` | unit with fake function pointers | ✅ | `unsafe.Pointer` buffers only (audited) |
| M13-03 | Verification: SDK layout check with clang (both arches); live comparison with `lsof` | `libproc_*darwin_test.go` | integration (macOS CI) | ✅ | Agreed on all 10 listening sockets here; identical `list -l` output |
| M13-04 | ADR-0003 and docs | `docs/adr`, permissions, architecture, troubleshooting | strict docs build | ✅ | Socket listing 43 ms → 4 ms; `list` warm ~20 ms → <10 ms |

## M14 — VS Code extension: MVP

Design: [ADR-0004](docs/adr/0004-vscode-extension.md). The extension is a thin client of
`portcheck --json`; all port logic stays in Go.

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M14-01 | Scaffold: strict TypeScript, esbuild bundle, ESLint, Vitest, npm lockfile | `extensions/vscode/` | `npm run check` | ✅ | TypeScript 6.0 (typescript-eslint doesn't support 7.x yet), strict + `exactOptionalPropertyTypes`; npm install scripts explicitly allowed/denied |
| M14-02 | CLI client: binary resolution (setting → bundled → PATH), run with timeout, JSON envelope + `schemaVersion` check, typed results | `src/cli.ts`, `src/schema.ts` | unit (fake exec), 100% | ✅ | Tests use the Go CLI's golden JSON, so schema drift breaks them |
| M14-03 | View model: group/sort sockets, labels, tooltips (pure, no `vscode` import) | `src/model.ts` | unit, 100% | ✅ | Merges IPv4/IPv6 listeners per owner; containers with hidden proxies handled |
| M14-04 | Ports sidebar: tree view, refresh while visible, context actions | `src/portsView.ts` | integration | ✅ | Refreshes only while visible; errors and "owners hidden" shown as view messages |
| M14-05 | Commands: check port, find free port (copy/insert), stop process (modal confirm, CLI guards), copy PID, open in browser | `src/commands.ts` | integration | ✅ | Container/WSL rows never call kill; container JSON gained `stopHint` (Go) so logic stays in one place |
| M14-06 | Integration tests in a real VS Code (`@vscode/test-electron`) + CI job (3 OS) | `src/test/`, `.github/workflows/vscode.yml` | CI green | ✅ | 15 tests in real VS Code 1.142 (macOS); CI on 3 OS + the minimum VS Code (1.95, raised to 1.98 in M15) |
| M14-07 | Docs: extension guide | `docs/guide/vscode.md` | strict docs build | ✅ | `docs/guide/vscode.md`, README, CONTRIBUTING, architecture |

## M15 — VS Code extension: USP

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M15-01 | "Port in use" matcher for Node, Vite, Next.js, Python, Go, Rails, Spring, … | `src/matcher.ts` | unit with a corpus of real error lines | ✅ | 40+ real lines (Node, Vite, Next, CRA, Angular, Bun, Deno, Go, Rust, Python/uvicorn/Flask/Django, Ruby, Spring, .NET, PHP, nginx, Redis, Postgres, Docker). Port-less errors take the port from the command or the tool's default; streamed output scanner strips ANSI. Minimum VS Code raised to 1.98: in 1.95–1.97 the terminal output API ends before the output arrives (verified per version) |
| M15-02 | Terminal link provider: matched lines become links to a fix menu | `src/terminal.ts` | integration | ✅ | Links on matched lines, plus a notification from live output via shell integration (`portcheck.terminal.notify`); tested in a real terminal |
| M15-03 | Fix menu: stop holder and retry, run on next free port, container/WSL commands, details | `src/fixes.ts`, `src/terminal.ts` | unit + integration | ✅ | Stop and re-run, re-run on next free port (port replaced, tool flag added, or `PORT` per shell), container/WSL/elevated commands, copy, details. Verified end to end: re-run on 3001 → HTTP 200; stop holder and re-run → HTTP 200 |
| M15-04 | Project ports: status bar (`doctor`), diagnostics for `.portcheck.yaml`, JSON Schema registration | `src/project.ts`, `src/projectModel.ts` | unit + integration | ✅ | Status bar + click-to-fix, Problems for failed checks and invalid files (with line numbers), project ports first in the view, schema bundled for the YAML extension. UDP hidden by default |
| M15-05 | Docs + demo recording | `docs/guide/vscode.md` | strict docs build | ✅ | Demo GIF recorded in a real VS Code (`docs/assets/portcheck-vscode-demo.gif`): npm run dev fails, notification, fix menu, stop and re-run. In the README, docs and the store page |

## M16 — VS Code extension: packaging & publishing

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M16-01 | VSIX with the matching `portcheck` binaries from the release | `release.yml` (`vscode` job), `extensions/vscode/scripts/package-release.sh` | VSIX smoke test | ✅ | One universal VSIX (27 MB) with `bin/<platform>-<arch>/` for all 6 targets; the extension picks its own at runtime. Chosen over 6 per-platform VSIX because the Marketplace website uploads one file at a time and the CLI token (Azure DevOps) isn't available. Binaries from the signed release (cosign + checksums checked first), provenance attested, attached to the release. Tested with v0.5.0 assets: installed in VS Code, picks `darwin-arm64`, executable, used without settings |
| M16-02 | Publish to VS Code Marketplace | release workflow | — | ✅ | [Live](https://marketplace.visualstudio.com/items?itemName=portcheck-vscode.portcheck-vscode) as `portcheck-vscode.portcheck-vscode`, "Portcheck - Port in Use Fixer". The first upload (`portcheck` under another publisher) was removed, which permanently reserves its name and title. Uploaded by hand (one VSIX per release): Azure DevOps sign-in failed for the maintainer and global PATs retire 2026-12-01; the workflow publishes automatically if `VSCE_PAT` is ever set |
| M16-03 | Publish to Open VSX (VSCodium, Cursor, Windsurf) | release workflow | — | ✅ | [Live](https://open-vsx.org/extension/portcheck-vscode/portcheck-vscode); the release job created the `portcheck-vscode` namespace and publishes every release. Namespace shows as unverified until it's claimed: the claim needs a GitHub account at least a year old (`roystondwork-dev`: October 2027) |
| M16-04 | Version sync with portcheck releases (release-please extra-files) | `release-please-config.json` | — | ✅ | `extra-files` bump `package.json` and `package-lock.json` (JSONPaths checked with release-please's jsonpath-plus); the packaging script fails if versions differ |

## M17 — Browser extension

Design: [ADR-0005](docs/adr/0005-browser-extension.md). portcheck itself is the native
messaging host; the extension can only ask for `version`, `list`, `check` and `kill`.

| ID | Task | Component / files | Tests required | Status | Notes |
|---|---|---|---|---|---|
| M17-01 | Native messaging host: framing, allowlisted operations run in-process with `--json` | `internal/nativehost/`, `internal/cli/browser.go` | unit 100%; real binary driven like Chrome | ✅ | Started with the extension origin as argument; guards apply (the browser is the host's parent, so it can't be stopped); 1 MiB answer limit handled |
| M17-02 | `portcheck browser install / uninstall / status` (Chrome, Edge, Chromium, Brave; macOS, Linux, Windows registry) | `internal/nativehost/install.go`, `registry_windows.go` | unit (fake + real registry on Windows CI) | ✅ | Per user, no admin; manifest `0600`; prefers the binary on `PATH` (survives upgrades); extension ID pinned: `fmocabgaaihiflefcdafnhoihcibgnhl` |
| M17-03 | Extension scaffold: Manifest V3, strict TypeScript, esbuild, ESLint, Vitest | `extensions/browser/` | `npm run check` | ✅ | Same toolchain as the VS Code extension; permissions: `nativeMessaging` + localhost URLs only (no tabs/history access) |
| M17-04 | Badge on `localhost:*` tabs: which process serves it | `src/background.ts` | unit 100% | ✅ | Badge per localhost tab; checks open tabs when the worker starts (events from before are lost: found by a stress test, 0/8 → 16/16) and ignores stale answers |
| M17-05 | Popup: process details, who started it, Stop (confirm), local ports list; setup help when portcheck isn't registered | `src/popup.ts` | unit 100% | ✅ | Inline confirmation (Stop / Force kill / Cancel); containers get their stop command; setup steps with copy buttons |
| M17-06 | "Connection refused" on localhost: say whether anything listens | `src/background.ts` | unit | ✅ | Via the badge: `off` on error pages such as ERR_CONNECTION_REFUSED |
| M17-07 | End-to-end test in real Chromium (Playwright) with the real binary + CI | `extensions/browser/e2e/`, `.github/workflows/browser.yml` | CI green | ✅ | 5 tests: badge, `off`, popup + real Stop, startup race, setup help; native host registered in the test profile. CI on Linux and macOS (Windows registration covered by Go tests) |
| M17-08 | Docs, release zip, store publishing (Edge Add-ons free; Chrome Web Store has a one-time fee) | `docs/guide/browser.md`, `release.yml` | strict docs build | 🟨 | Docs ✅; release zip ✅ and store zip (`portcheck-browser-store-X.Y.Z.zip`, no manifest key) ✅. **Edge Add-ons** (free): listing kit in `extensions/browser/store/` (text, privacy answers, tester notes, images rendered from the real extension), privacy policy page ✅; maintainer submits after v0.7.0 (testers need `portcheck browser`), then add the store ID to `nativehost.ExtensionIDs`. Until then, store builds show the `--extension-id` command for their own ID (e2e-tested). Chrome Web Store deferred until ~100 users |

## What's next (candidates, not yet scheduled)

| Candidate | Notes |
|---|---|
| Announce v0.2.0 (r/golang) | Draft prepared; maintainer posts |
| Community setup | Labels, Project board, OpenSSF badge |
| More channels | AUR, Nix, Docker image |

---

## Decisions log

| Date | Decision | Ref |
|---|---|---|
| 2026-10-20 | Go CLI + Bubble Tea TUI; MIT license; name `portcheck` | plan |
| 2026-10-20 | gopsutil behind `inspect.Provider` | [ADR-0001](docs/adr/0001-gopsutil-provider.md) |
| 2026-10-20 | Scanner is TCP-connect only, no stealth features | [ADR-0002](docs/adr/0002-tcp-connect-only-scanning.md) |
| 2026-10-20 | Project config is `.portcheck.yaml` (found upward), strict, with a published JSON Schema | M12 |
| 2026-10-20 | `run` exits with the child's status; Ctrl+C is not relayed (the terminal delivers it), SIGTERM/SIGHUP are | M9 |
| 2026-10-20 | Container attribution via the Engine API (no Docker SDK); `kill` never stops container proxies | M8 |
| 2026-10-20 | winget package ID `roystondwork-dev.portcheck` (chosen by maintainer; permanent once accepted) | #10 |
| 2026-10-20 | `wait` checks TCP accept, not application health; documented | #11 |
| 2026-10-20 | First release is 0.1.0 (`initial-version`), not release-please's default 1.0.0 | #6 |
| 2026-10-20 | TUI uses a custom table + golden screen snapshots instead of bubbles/table + teatest | M4 notes |
| 2026-10-20 | macOS sockets via libproc + purego, `lsof` as fallback | [ADR-0003](docs/adr/0003-native-macos-sockets.md) |
| 2026-10-20 | VS Code extension first, as a thin client of the CLI's JSON; browser extension optional | [ADR-0004](docs/adr/0004-vscode-extension.md) |
| 2026-10-20 | Browser extension via native messaging, portcheck itself as host, four allowlisted operations | [ADR-0005](docs/adr/0005-browser-extension.md) |
