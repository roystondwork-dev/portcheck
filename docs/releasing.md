# Releasing

Releases are automated. Maintainers only merge a pull request.

```mermaid
flowchart LR
    A["feat/fix commits<br/>merged to main"] --> B["release-please updates<br/>'chore: release x.y.z' PR"]
    B -->|maintainer merges| C["tag vX.Y.Z +<br/>GitHub release with notes"]
    C --> D["GoReleaser: build 6 targets,<br/>archives, deb/rpm/apk"]
    D --> E["SBOMs (syft) +<br/>cosign-signed checksums"]
    E --> F["Homebrew cask + Scoop manifest<br/>+ winget PR + SLSA provenance"]
```

## How versions are chosen

[release-please](https://github.com/googleapis/release-please) reads the
[Conventional Commit](https://www.conventionalcommits.org/) titles merged since the
last release:

| Commits since last release | Before 1.0.0 | From 1.0.0 |
|---|---|---|
| only `fix:` / `perf:` | patch | patch |
| any `feat:` | **minor** | minor |
| any `!` / `BREAKING CHANGE:` | **minor** | **major** |

`CHANGELOG.md` and the release notes are generated from the same commits; `docs:`
and `build:` commits are listed too, while `test:`, `ci:`, `refactor:` and `chore:`
are hidden.

## Cutting a release

1. Merge the open **"chore: release x.y.z"** pull request after reviewing its changelog.
2. The **Release** workflow tags `vX.Y.Z`, creates the GitHub release and runs GoReleaser.
3. Check that the release has its archives, packages, `checksums.txt`,
   `checksums.txt.sigstore.json` and `*.sbom.json` files, and that the
   [tap](https://github.com/roystondwork-dev/homebrew-tap) and
   [bucket](https://github.com/roystondwork-dev/scoop-bucket) received commits.
4. Smoke-test: `brew install roystondwork-dev/tap/portcheck && portcheck version`.
5. Check that the release has `portcheck-vscode-X.Y.Z.vsix`. The **vscode** job
   verifies the signed `checksums.txt`, puts the binary for every platform into one
   extension package (`bin/<platform>-<arch>/`; the extension picks its own at runtime),
   attaches it, and publishes it to the VS Code Marketplace and Open VSX when their
   tokens are set. Without `VSCE_PAT`, upload that one file on
   [the Marketplace's management page](https://marketplace.visualstudio.com/manage)
   (**⋯ → Update**). The extension's version comes
   from the same release PR (`extra-files` in `release-please-config.json`).
6. Check that the release has the browser extension, built by the **browser** job (its
   version also comes from the release PR): `portcheck-browser-X.Y.Z.zip` (load
   unpacked), `portcheck-browser-store-X.Y.Z.zip` (Edge Add-ons),
   `portcheck-firefox-X.Y.Z.zip` (Firefox Add-ons), `portcheck-firefox.xpi` and
   `portcheck-firefox-X.Y.Z.xpi` (signed by Mozilla for direct installs) and
   `portcheck-firefox-source-X.Y.Z.zip` (the source Firefox's reviewers rebuild; see
   `extensions/browser/BUILD-FIREFOX.md`). The job also **submits the new version to
   both stores**, which then review it (a few days):

   - Firefox Add-ons, with `web-ext sign --channel listed` and the listing in
     `extensions/browser/store/amo-metadata.json`. Secrets: `AMO_JWT_ISSUER`,
     `AMO_JWT_SECRET` (addons.mozilla.org → Tools → Manage API Keys).
   - Microsoft Edge Add-ons, with `scripts/edge-publish.sh`. Secrets: `EDGE_CLIENT_ID`,
     `EDGE_API_KEY`, `EDGE_PRODUCT_ID` (Partner Center → Microsoft Edge → Publish API).

   Each step is skipped without its secrets, and a failed submission doesn't fail the
   release: check the job's log, and upload by hand if needed (the zip above; listing
   texts in `extensions/browser/store/`). The Edge API key expires after 72 days; create
   a new one in Partner Center and update `EDGE_API_KEY` when it does.
7. **winget submissions are paused** while the first one,
   [microsoft/winget-pkgs#457169](https://github.com/microsoft/winget-pkgs/pull/457169)
   (0.8.0), waits for a Microsoft moderator: each release would otherwise open a new PR
   and lose its place in the queue. The release skips winget unless the repository
   variable `WINGET_SUBMIT` is `true`. Once the package is accepted, resume with
   `gh variable set WINGET_SUBMIT --body true --repo roystondwork-dev/portcheck`;
   after that, follow the PR each release opens on
   [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs/pulls?q=is%3Apr+roystondwork-dev.portcheck)
   (comment the CLA agreement if asked) and keep the fork's `master` branch in sync
   occasionally (`gh repo sync roystondwork-dev/winget-pkgs`).

To rebuild artifacts for an existing tag (for example, after fixing a publishing
problem), run the **Release** workflow manually and enter the tag. Tick **vscode_only**
to only package and publish the VS Code extension: the extension from the branch the
workflow runs on (usually `main`), with the portcheck binaries from that tag's release.
Its `package.json` version must equal the tag's.

## Repository secrets

| Secret | Required | Purpose |
|---|---|---|
| `HOMEBREW_TAP_GITHUB_TOKEN` | for Homebrew/Scoop | Fine-grained PAT with **Contents: read & write** on `homebrew-tap` and `scoop-bucket`. Without it, those publishers are skipped with a warning. |
| `WINGET_GITHUB_TOKEN` | for winget | **Classic** PAT with the `public_repo` scope. GoReleaser pushes the manifest to the [`winget-pkgs` fork](https://github.com/roystondwork-dev/winget-pkgs) and opens a PR to `microsoft/winget-pkgs`; fine-grained tokens can't open PRs on repositories you don't own. Without it, the winget submission is skipped with a warning. Submissions also need the repository **variable** `WINGET_SUBMIT=true` (paused for now, see above). |
| `RELEASE_PLEASE_TOKEN` | recommended | Fine-grained PAT with **Contents** and **Pull requests: read & write** on this repo. PRs opened with the default `GITHUB_TOKEN` don't trigger CI, so required checks never run on the release PR. Without this token, a maintainer must merge the release PR with admin override. |
| `VSCE_PAT` | for the VS Code Marketplace | Azure DevOps personal access token with the **Marketplace: Manage** scope for *All accessible organizations*, from the account that owns the `portcheck-vscode` publisher. Without it, Marketplace publishing is skipped with a warning. |
| `OVSX_PAT` | for Open VSX | Access token from [open-vsx.org](https://open-vsx.org) (user settings), for the account that owns the `portcheck-vscode` namespace and has signed the publisher agreement. Without it, Open VSX publishing is skipped with a warning. |
| `CODECOV_TOKEN` | optional | Coverage upload; the coverage gate doesn't depend on it. |

!!! note "Without `RELEASE_PLEASE_TOKEN`"
    New repositories don't allow workflows to open pull requests with the default
    token, so release-please fails with *"GitHub Actions is not permitted to create or
    approve pull requests"*. Either set `RELEASE_PLEASE_TOKEN` (recommended), or enable
    *Settings → Actions → General → Allow GitHub Actions to create and approve pull requests*.

The first release is `0.1.0` (`initial-version` in `release-please-config.json`).

Signing needs no secret: cosign uses the workflow's GitHub OIDC identity
(keyless), and users verify against that identity; see
[verifying downloads](install.md#verifying-downloads).

npm, PyPI and the [MCP Registry](https://registry.modelcontextprotocol.io) need no
secret either: each logs in with the same OIDC identity (trusted publishing). The MCP
Registry entry is `server.json` at the repository root (`io.github.roystondwork-dev/portcheck`,
pointing at `npx portcheck-cli mcp`); release-please bumps its versions, and the
registry checks them against `mcpName` in the published npm package, so it's
published right after npm.

## Local dry run

```sh
make snapshot   # builds everything into dist/ without publishing or signing
```

This needs `goreleaser` and, for SBOMs, `syft` (`brew install goreleaser syft`).
