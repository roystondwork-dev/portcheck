# ADR-0006: A JetBrains plugin as a thin client of the portcheck CLI

- **Status:** Accepted
- **Date:** 2026-10-24

## Context

Many developers run their dev servers from IntelliJ-based IDEs (IntelliJ IDEA,
WebStorm, PyCharm, GoLand, Rider, …), where "port already in use" shows up in a
Run/Debug console. The VS Code extension ([ADR-0004](0004-vscode-extension.md)) solved
the same problem for VS Code's terminal.

## Decision

1. **Same architecture as the VS Code extension:** a Kotlin plugin that runs
   `portcheck … --json` and reads schema v1. No port logic is reimplemented, so
   `kill`'s guards apply (the IDE is portcheck's parent, so it can't be stopped).
2. **portcheck is bundled:** the release plugin zip carries the binary for all six
   platforms in `bin/<os>-<arch>/`, verified against the release's signed checksums,
   like the VSIX. A setting or `PATH` overrides it.
3. **The fix lives where the error appears:** an `ExecutionListener` watches every
   Run/Debug process's output with the same patterns as the VS Code matcher (plus Java
   servers: Tomcat, `BindException` with Spring Boot's default port). A notification
   offers **Stop … and rerun**, which restarts the same execution environment.
4. **Platform:** Kotlin, the IntelliJ Platform Gradle Plugin 2.x, built against
   2025.2 and compatible with later releases (no until-build); the Plugin Verifier
   checks current IDEs in CI.
5. **Distribution:** every release attaches `portcheck-jetbrains-X.Y.Z.zip`; JetBrains
   Marketplace (free) gets each version through its upload API once the first version
   is listed by hand.

## Consequences

- Two matchers (TypeScript and Kotlin) must stay in sync; both test the same corpus of
  real error lines.
- IDEs older than 2025.2 aren't supported.
- The plugin zip is ~29 MB because of the six binaries, like the VSIX.
