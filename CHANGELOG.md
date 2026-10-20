# Changelog

## [0.2.0](https://github.com/roystondwork-dev/portcheck/compare/v0.1.1...v0.2.0) (2026-10-20)


### Features

* **cli:** add wait command ([436594b](https://github.com/roystondwork-dev/portcheck/commit/436594b934b617a6384cada5fb442937fc7c27c2))
* **containers:** attribute published ports to Docker and Podman containers ([42b18e9](https://github.com/roystondwork-dev/portcheck/commit/42b18e9b15dbc4f14043a69627b5f0d6f6a7f111))
* show containers instead of proxy processes; never kill Docker infrastructure ([0b5ddac](https://github.com/roystondwork-dev/portcheck/commit/0b5ddacb5d21414a5c245b338e1b9434c1b9460e))
* **waitfor:** add polling library for waiting on ports ([79a8357](https://github.com/roystondwork-dev/portcheck/commit/79a83572aae5fb1cf43436d7613cea68ea1cdd39))


### Documentation

* document container attribution ([1d5f3be](https://github.com/roystondwork-dev/portcheck/commit/1d5f3be068722d92a15a0bd91747cdefb5696968))
* fix inaccuracies found in a full docs audit; honor --no-color in the TUI ([ebd3b0f](https://github.com/roystondwork-dev/portcheck/commit/ebd3b0fc3ca45d1146685dce86e161fe14fd00a2))
* **tasks:** record post-release work (M7), the docs audit and the Docker plan (M8) ([725e3d1](https://github.com/roystondwork-dev/portcheck/commit/725e3d189e6523df530b6229ef8d7edd0a1ba6ac))

## [0.1.1](https://github.com/roystondwork-dev/portcheck/compare/v0.1.0...v0.1.1) (2026-10-20)


### Documentation

* correct macOS low-port binding and tidy post-release notes ([b1239dd](https://github.com/roystondwork-dev/portcheck/commit/b1239dd41fbf62188e96f1de5c297971f342572f))


### Build & Release

* **release:** publish to winget ([eda703b](https://github.com/roystondwork-dev/portcheck/commit/eda703b2a680c2238a5d7cca0687b57b7c89f994))
* **release:** use roystondwork-dev.portcheck as the winget package ID ([0bfacee](https://github.com/roystondwork-dev/portcheck/commit/0bfaceefd04d6cdd1daae93e2e833edd3897643e))

## 0.1.0 (2026-10-20)


### Features

* **cli:** add list, check, free, kill and version commands (M2) ([2cb0f33](https://github.com/roystondwork-dev/portcheck/commit/2cb0f33bb286cc99aa40fcfb9aa59a0e148d4953))
* **cli:** add scan command (M3) ([df97c4e](https://github.com/roystondwork-dev/portcheck/commit/df97c4ecdfaa5a6c338f0e96dc723d9472c5c1a0))
* **cli:** add tui command and open it by default in a terminal (M4) ([a8708c9](https://github.com/roystondwork-dev/portcheck/commit/a8708c91df7b04fd3c515a4d816afd50501691ea))
* **pkg:** add portspec, inspect, freeport and killer libraries (M1) ([64e95f7](https://github.com/roystondwork-dev/portcheck/commit/64e95f7edc06802addc11f59f227674fe7207c1e))
* **scan:** add TCP connect scanner library ([407b113](https://github.com/roystondwork-dev/portcheck/commit/407b113f18910a4883c84a59ab582a4b84af7efe))
* **tui:** add interactive live view of ports ([a2d979d](https://github.com/roystondwork-dev/portcheck/commit/a2d979d11e86730748b8e1b49922499979990c9e))


### Bug Fixes

* **inspect:** treat remote port 0 as unconnected on Linux and Windows ([972f996](https://github.com/roystondwork-dev/portcheck/commit/972f9967a6fc02779d0e1405925e0a3e064dda35))


### Documentation

* add documentation site, guides, examples and demo (M5) ([8a0ff22](https://github.com/roystondwork-dev/portcheck/commit/8a0ff224457c95bb33c62f7f3698ac7b1a3498a5))
* **tasks:** mark M0 complete ([70ccd5d](https://github.com/roystondwork-dev/portcheck/commit/70ccd5dbaae0fde85f51e9049525396ab8f99a2a))


### Build & Release

* automate releases with release-please and GoReleaser (M6) ([c651b0a](https://github.com/roystondwork-dev/portcheck/commit/c651b0a28612b4b007daf5b3f885b0339c800635))
* bootstrap project tooling, CI and community files (M0) ([348dc08](https://github.com/roystondwork-dev/portcheck/commit/348dc08dbf890a37bca43a08b58cfee918732aae))
* **release:** start versioning at 0.1.0 ([0de88ac](https://github.com/roystondwork-dev/portcheck/commit/0de88ac9a934859c67be56b9959f71f012e5f0f2))
