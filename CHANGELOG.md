# Changelog

## [0.10.0](https://github.com/roystondwork-dev/portcheck/compare/v0.9.0...v0.10.0) (2026-10-24)


### Features

* **jetbrains:** JetBrains plugin: fix "port in use" from Run/Debug ([0a225bd](https://github.com/roystondwork-dev/portcheck/commit/0a225bda15bf77647011a6ba0b3b560b5e25d7f9))
* **mcp:** portcheck mcp, an MCP server for AI coding assistants ([456f389](https://github.com/roystondwork-dev/portcheck/commit/456f3892aad1ab55da9fa2045df6b64e07d2dcae))
* **neovim:** Neovim plugin: pick, check, kill and free ports; :terminal watcher ([103b2c4](https://github.com/roystondwork-dev/portcheck/commit/103b2c419793a7a49d4995d1a05d5c4ab02dcefe))
* **packaging:** npm and PyPI packages (npx portcheck-cli, pipx install portcheck-tool) ([#52](https://github.com/roystondwork-dev/portcheck/issues/52)) ([0bf63bb](https://github.com/roystondwork-dev/portcheck/commit/0bf63bbf553d69fcc1a9ccb088d22e55feb1cd06))
* **raycast:** Raycast extension: search, check, kill and free ports ([414fb73](https://github.com/roystondwork-dev/portcheck/commit/414fb73091a890dec64f1b15c35e97280a578b6b))
* **tray:** portcheck-tray, a menu bar and system tray app ([a511778](https://github.com/roystondwork-dev/portcheck/commit/a51177827f251a74ec5798f9c58e1c5f3d243b2c))
* **zed:** Zed extension: portcheck's MCP tools in Zed's agent ([52aec24](https://github.com/roystondwork-dev/portcheck/commit/52aec243cde023149d89fd495ed73475dd944e6c))


### Bug Fixes

* **jetbrains:** find the plugin's directory with public API; Windows test ([d7016da](https://github.com/roystondwork-dev/portcheck/commit/d7016da52737e6babc5592c7a97d2b249f62d0c6))
* **jetbrains:** locate the bundled portcheck from the plugin's own jar ([0eba2c2](https://github.com/roystondwork-dev/portcheck/commit/0eba2c2d7e4c00021f6b4e229403258afeab8673))
* **security:** update vulnerable dev dependencies; pin docs deps by hash; drop the Gradle wrapper JAR ([6779cf5](https://github.com/roystondwork-dev/portcheck/commit/6779cf562376ebabdbe85cf4ff893342ac8b9390))
* **vscode:** re-check project ports after a change during a check ([bacd2f3](https://github.com/roystondwork-dev/portcheck/commit/bacd2f319d11708773d237ef22a2296b4690c8af))


### Documentation

* **jetbrains:** link the demo video ([b248db5](https://github.com/roystondwork-dev/portcheck/commit/b248db5ce87fbf7fe72d45a6cefae2896d0edb9b))
* **jetbrains:** screenshots of the plugin in IntelliJ IDEA ([558c03d](https://github.com/roystondwork-dev/portcheck/commit/558c03d5874b7e2383f8928504aa8ad2e5710806))
* **mcp:** add Zed's server through its UI ([f8b1239](https://github.com/roystondwork-dev/portcheck/commit/f8b12395f857b278c87154a9b2ea7d865bc964cf))
* OpenSSF Best Practices passing badge ([6dafc1f](https://github.com/roystondwork-dev/portcheck/commit/6dafc1f15c0c579dbcb370816b450c7a2ffef425))
* **raycast:** store screenshots (Search Ports, Check Port) ([3811c4a](https://github.com/roystondwork-dev/portcheck/commit/3811c4ada038d90eb18c36781dd31a7ca1fbfbed))

## [0.9.0](https://github.com/roystondwork-dev/portcheck/compare/v0.8.0...v0.9.0) (2026-10-23)


### Features

* **browser:** Firefox build of the extension ([1aeb6bc](https://github.com/roystondwork-dev/portcheck/commit/1aeb6bcfa3311293021fed740dac3775fff00cec))
* **cli:** register portcheck with Firefox ([1378f0a](https://github.com/roystondwork-dev/portcheck/commit/1378f0a983f55ddfbe101391ab645a2c4fb66efb))
* one-command setup, and an extension that works before it ([ae5715b](https://github.com/roystondwork-dev/portcheck/commit/ae5715b36f746bc07876332846190bcdad13df5a))
* **release:** attach the Firefox add-on and its source to releases ([43a8c43](https://github.com/roystondwork-dev/portcheck/commit/43a8c43f1f399f988b073ac4d5747d218bf32c0a))
* **release:** Mozilla-signed Firefox download that updates itself ([965437f](https://github.com/roystondwork-dev/portcheck/commit/965437f1d504e206a062a4c58651b66ae78ae192))


### Bug Fixes

* **install:** find the latest release without GitHub's API ([326c2d1](https://github.com/roystondwork-dev/portcheck/commit/326c2d1391f95e7b3d576f40b42e158a52296aa9))
* **install:** read checksums as text on Windows; test: grant localhost access in Firefox ([5f8e681](https://github.com/roystondwork-dev/portcheck/commit/5f8e6817d1f98e526227546276fac3e7f800b51d))
* **release:** use Homebrew install steps for the cask's quarantine hook ([0939115](https://github.com/roystondwork-dev/portcheck/commit/0939115c2d0a7e3a239586bac1f124ba2921972b))


### Documentation

* **browser:** Firefox Add-ons listing kit ([1467be5](https://github.com/roystondwork-dev/portcheck/commit/1467be52b42829c8b969a69b5a9a6981fe2ee492))
* **browser:** one-command setup in the browser guide; Firefox in the intro ([f6433e9](https://github.com/roystondwork-dev/portcheck/commit/f6433e94395dd8320a3cac59106860289761804b))
* **tasks:** plan Safari (M20) for after 100 users ([edb6cfd](https://github.com/roystondwork-dev/portcheck/commit/edb6cfd68f1e9999902259d20cb13fe75915de14))
* the browser extension is on Microsoft Edge Add-ons ([262fdb3](https://github.com/roystondwork-dev/portcheck/commit/262fdb33546937e1b30a1be99856b068659d7501))
* the macOS checks are required again ([9ee2674](https://github.com/roystondwork-dev/portcheck/commit/9ee2674f3678d3159a17598fe45846a0486b9ddd))
* winget 0.8.0 submission; latest release v0.8.0 ([58925af](https://github.com/roystondwork-dev/portcheck/commit/58925af8b52df3fa2fd65ecda48c2c56f8c598cc))

## [0.8.0](https://github.com/roystondwork-dev/portcheck/compare/v0.7.0...v0.8.0) (2026-10-20)


### Features

* **browser:** allow the Edge Add-ons build by default ([a60651c](https://github.com/roystondwork-dev/portcheck/commit/a60651ccf2384f227e69233e979f0fe8befc2231))

## [0.7.0](https://github.com/roystondwork-dev/portcheck/compare/v0.6.0...v0.7.0) (2026-10-20)


### Features

* **browser:** Chrome/Edge extension showing which process serves a localhost page ([41f985f](https://github.com/roystondwork-dev/portcheck/commit/41f985f158a8dbd202894461e857ac0382af8433))
* **browser:** Edge Add-ons kit and store builds ([d9f41e3](https://github.com/roystondwork-dev/portcheck/commit/d9f41e3b6bdbc5497dca4304bd83835c11c3c7e4))
* **browser:** redesign the popup ([c202419](https://github.com/roystondwork-dev/portcheck/commit/c20241945a50bf2793dbed9be1bc5af06d2dbe70))
* **cli:** portcheck browser install, status and uninstall; native host mode ([1f6b8aa](https://github.com/roystondwork-dev/portcheck/commit/1f6b8aa205a490b45a90f725fd4d0a59b3906b2b))
* **nativehost:** native messaging for the browser extension ([7f11003](https://github.com/roystondwork-dev/portcheck/commit/7f11003d46bd61eb32e849e8403dc6841bbc903f))
* **release:** attach the browser extension to every release ([1296eb2](https://github.com/roystondwork-dev/portcheck/commit/1296eb2119ab596f1494132b26a6a76bf6cf0564))


### Documentation

* browser commands, JSON schema and M17 tasks ([208ab79](https://github.com/roystondwork-dev/portcheck/commit/208ab794072b0d284344dbe3a7e377ce8bfe4d8b))
* browser extension guide; M17 status ([2d4c9da](https://github.com/roystondwork-dev/portcheck/commit/2d4c9daf50620df139bbe8a601f2aa130a1d8857))
* browser extension troubleshooting, incl. the macOS folder prompt ([f8f0365](https://github.com/roystondwork-dev/portcheck/commit/f8f03652d78909b9474d100aeb743e93c5454430))
* defer the Chrome Web Store until portcheck has 100 users ([1ba4894](https://github.com/roystondwork-dev/portcheck/commit/1ba4894e13b7b100ab053c66e9b41e3e4862cb0d))
* plan the browser extension (ADR-0005) ([82c6598](https://github.com/roystondwork-dev/portcheck/commit/82c65984ffe5f76039176f3dff28a5451627332e))

## [0.6.0](https://github.com/roystondwork-dev/portcheck/compare/v0.5.0...v0.6.0) (2026-10-20)


### Features

* **vscode:** add an icon and Marketplace banner colour ([64e4876](https://github.com/roystondwork-dev/portcheck/commit/64e4876f2f9d3130cc3e83fb50a87bec75107d33))
* **vscode:** expose the extension's own UI in its API ([034c379](https://github.com/roystondwork-dev/portcheck/commit/034c3797a1ff5d289b1feb1e8eb5011c48067482))
* **vscode:** package the extension per platform with portcheck built in ([f214b84](https://github.com/roystondwork-dev/portcheck/commit/f214b842a240c1a6bffa2efa371097b4509cabcf))
* **vscode:** ship one VSIX with portcheck for every platform ([1ae0768](https://github.com/roystondwork-dev/portcheck/commit/1ae0768348f8339ef46b0e93a03268ea7321df2f))


### Bug Fixes

* **vscode:** new display title, Portcheck - Port in Use Fixer ([789d1cd](https://github.com/roystondwork-dev/portcheck/commit/789d1cd2a365dc5d993d09452edf1b125c941328))
* **vscode:** publish under the portcheck-vscode publisher ([7bbb019](https://github.com/roystondwork-dev/portcheck/commit/7bbb019686e228f9de2c24ccd408e78c6ddb5b72))
* **vscode:** rename the extension to portcheck-vscode ([033936c](https://github.com/roystondwork-dev/portcheck/commit/033936ce86a1dae017594185ba49857c755cc838))


### Documentation

* installing the VS Code extension from a release; M16 status ([7d6299e](https://github.com/roystondwork-dev/portcheck/commit/7d6299eeec9a9551c5d56b98cdfaf629e8d387a7))
* note the temporarily optional macOS checks ([deeeacb](https://github.com/roystondwork-dev/portcheck/commit/deeeacb900e3093c2968b8f6728e9bfe6ab06643))
* VS Code extension demo, Marketplace and Open VSX links; M15 and M16 done ([1258d41](https://github.com/roystondwork-dev/portcheck/commit/1258d41cca009257c83cd1fa053493499d72fc45))

## [0.5.0](https://github.com/roystondwork-dev/portcheck/compare/v0.4.0...v0.5.0) (2026-10-20)


### Features

* **output:** include the stop command for container and WSL ports in JSON ([7caada7](https://github.com/roystondwork-dev/portcheck/commit/7caada7a66242188ddfeba91500fc2ec00feb3df))
* **vscode:** add the VS Code extension (MVP) ([bc18b62](https://github.com/roystondwork-dev/portcheck/commit/bc18b62fe59dcc513809b7377c06e82a8e34b79e))
* **vscode:** fix busy ports from the terminal; check project ports ([719c879](https://github.com/roystondwork-dev/portcheck/commit/719c87999b8ec7c9a538e0a12ee692f9b4e7b9b4))
* **vscode:** recognise "port in use" errors in terminal output ([ae676ce](https://github.com/roystondwork-dev/portcheck/commit/ae676cec4f31d23d64d44fddd7fca5c82bb80ef1))


### Bug Fixes

* **vscode:** require VS Code 1.98 for terminal error detection ([e185ed8](https://github.com/roystondwork-dev/portcheck/commit/e185ed8a745b3f3cef43e436e79501f83c68d87c))
* **vscode:** run integration tests on older VS Code and cover exec errors on Windows ([1d4f532](https://github.com/roystondwork-dev/portcheck/commit/1d4f532f3bc3c92038db7c810f8c61889524f3e7))


### Documentation

* plan the VS Code extension (ADR-0004, M14–M17) ([beb3f34](https://github.com/roystondwork-dev/portcheck/commit/beb3f34a7542f7d648a856e62f79f6b116229e3e))
* terminal fixes and project ports in the VS Code guide; M15 status ([333bf23](https://github.com/roystondwork-dev/portcheck/commit/333bf239bbab16a2e784f66a1cfb22100ff10867))
* update the winget submission status ([39f0f80](https://github.com/roystondwork-dev/portcheck/commit/39f0f805124914b0867e3b38fc8b4f78398d1e50))
* VS Code extension guide; M14 complete ([6317b50](https://github.com/roystondwork-dev/portcheck/commit/6317b50ee39441dc76ead9846f6ed231e779ee2e))


### Build & Release

* **deps:** bump github.com/ebitengine/purego ([18bc905](https://github.com/roystondwork-dev/portcheck/commit/18bc905f2c6fe5e5372784195a5bada7a8d08ac7))
* keep Go tooling out of extensions/ ([a2071d7](https://github.com/roystondwork-dev/portcheck/commit/a2071d7e2ec0983d8beb9efd8ef8cf8b13dc4ee6))
* **vscode:** load the Vitest config as ESM ([dd0f1b6](https://github.com/roystondwork-dev/portcheck/commit/dd0f1b621ef6da69f4e22c4541fb38f2e6d8b64b))

## [0.4.0](https://github.com/roystondwork-dev/portcheck/compare/v0.3.0...v0.4.0) (2026-10-20)


### Features

* add .portcheck.yaml and portcheck doctor ([cfe1a06](https://github.com/roystondwork-dev/portcheck/commit/cfe1a06355745fb69bc1aa171603f49e99e49c87))
* attribute WSL-forwarded ports to the distro and Linux process ([6733709](https://github.com/roystondwork-dev/portcheck/commit/6733709b8b7538024b2976f84f63cffabe392526))
* show who started a process (list --wide, kill confirmation, TUI) ([857bb3b](https://github.com/roystondwork-dev/portcheck/commit/857bb3b9dfe61b79584b2232932f94163ca3c60f))


### Performance

* **inspect:** read macOS sockets through libproc instead of running lsof ([dae4e7a](https://github.com/roystondwork-dev/portcheck/commit/dae4e7a7516952f18d0bb11cddeb3fe7825f7b33))


### Documentation

* ADR-0003 native macOS sockets; update permissions, architecture and tasks ([a6fd20f](https://github.com/roystondwork-dev/portcheck/commit/a6fd20fffea7682efe1ee3c9322f8ee0d60770c0))
* document .portcheck.yaml and doctor; publish the config JSON Schema ([220d743](https://github.com/roystondwork-dev/portcheck/commit/220d74356f1087dbcd32c94f7ea0b9af7c9fbd4d))
* **tasks:** M10–M13 complete ([adb9fc5](https://github.com/roystondwork-dev/portcheck/commit/adb9fc55224f1ae3455b9af348ca7736e03fcaf2))

## [0.3.0](https://github.com/roystondwork-dev/portcheck/compare/v0.2.0...v0.3.0) (2026-10-20)


### Features

* **cli:** add run command ([1f54993](https://github.com/roystondwork-dev/portcheck/commit/1f549934fd3ea6512391fd88661e78eeeeffb916))


### Documentation

* document portcheck run ([ef7dd76](https://github.com/roystondwork-dev/portcheck/commit/ef7dd76260ea6284c69ff341eef2ae6421d47da3))

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
