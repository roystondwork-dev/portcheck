# Install

portcheck is a single static binary with no runtime dependencies.

## One command

Installs portcheck and connects it to your browsers for the
[browser extension](guide/browser.md):

=== "macOS, Linux"

    ```sh
    curl -fsSL https://roystondwork-dev.github.io/portcheck/install.sh | sh
    ```

    With Homebrew, it installs the Homebrew package. Otherwise it downloads the latest
    release, checks it against the release's checksums, and installs it to
    `~/.local/bin` (`PORTCHECK_INSTALL_DIR=/somewhere` to choose).
    [Read the script](https://github.com/roystondwork-dev/portcheck/blob/main/docs/install.sh).

=== "Windows (PowerShell)"

    ```powershell
    irm https://roystondwork-dev.github.io/portcheck/install.ps1 | iex
    ```

    It downloads the latest release, checks it against the release's checksums,
    installs it to `%LOCALAPPDATA%\Programs\portcheck` and adds that folder to your
    user `PATH`. No administrator rights are needed.
    [Read the script](https://github.com/roystondwork-dev/portcheck/blob/main/docs/install.ps1).

Both finish with `portcheck browser install`; set `PORTCHECK_NO_BROWSER=1` to skip it,
or `PORTCHECK_VERSION=0.9.0` for a specific version.

## Package managers

=== "Homebrew (macOS, Linux)"

    ```sh
    brew install roystondwork-dev/tap/portcheck
    ```

=== "winget (Windows)"

    ```powershell
    winget install roystondwork-dev.portcheck
    ```

    Open a new terminal afterwards so `portcheck` is on your `PATH`.

    !!! note
        The package is waiting for review in
        [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs/pulls?q=is%3Apr+roystondwork-dev.portcheck).
        Until it's merged, use Scoop or the zip from the
        [releases page](https://github.com/roystondwork-dev/portcheck/releases).

=== "Scoop (Windows)"

    ```powershell
    scoop bucket add portcheck https://github.com/roystondwork-dev/scoop-bucket
    scoop install portcheck
    ```

=== "Linux packages"

    Download the `.deb`, `.rpm` or `.apk` for your architecture from the
    [latest release](https://github.com/roystondwork-dev/portcheck/releases/latest), then:

    ```sh
    sudo apt install ./portcheck_*_amd64.deb            # Debian, Ubuntu (arm64: *_arm64.deb)
    sudo dnf install ./portcheck-*.x86_64.rpm           # Fedora, RHEL (arm64: *.aarch64.rpm)
    sudo apk add --allow-untrusted ./portcheck_*_x86_64.apk  # Alpine (arm64: *_aarch64.apk)
    ```

=== "Binary archive"

    Download the archive for your OS and CPU from the
    [latest release](https://github.com/roystondwork-dev/portcheck/releases/latest),
    extract it, and put `portcheck` (`portcheck.exe` on Windows) on your `PATH`.

=== "Go"

    Requires Go 1.26 or newer:

    ```sh
    go install github.com/roystondwork-dev/portcheck/cmd/portcheck@latest
    ```

Check the installation:

```sh
portcheck version
man portcheck      # man pages ship with Homebrew and the Linux packages
```

## Shell completion

```sh
portcheck completion bash > /etc/bash_completion.d/portcheck          # bash
portcheck completion zsh  > "${fpath[1]}/_portcheck"                  # zsh
portcheck completion fish > ~/.config/fish/completions/portcheck.fish # fish
portcheck completion powershell | Out-String | Invoke-Expression      # PowerShell (current session)
```

Homebrew installs completions for you.

## Verifying downloads

Every release publishes `checksums.txt`, signed with [Sigstore cosign](https://docs.sigstore.dev/)
using GitHub's OIDC identity (no long-lived keys). It also publishes SPDX SBOMs and
[SLSA build provenance](https://slsa.dev/).

1. Verify that the checksum file was produced by this repository's release workflow:

    ```sh
    cosign verify-blob \
      --bundle checksums.txt.sigstore.json \
      --certificate-identity 'https://github.com/roystondwork-dev/portcheck/.github/workflows/release.yml@refs/heads/main' \
      --certificate-oidc-issuer https://token.actions.githubusercontent.com \
      checksums.txt
    ```

2. Verify your download against it:

    ```sh
    sha256sum --ignore-missing -c checksums.txt     # Linux
    shasum -a 256 --ignore-missing -c checksums.txt # macOS
    ```

3. Optionally, verify the build provenance with the GitHub CLI:

    ```sh
    gh attestation verify portcheck_*_linux_amd64.tar.gz --repo roystondwork-dev/portcheck
    ```

## Running with elevated privileges

Some information is only visible to root or Administrator; see
[Permissions by OS](permissions.md). `sudo` often resets `PATH`, so pass the full path:

```sh
sudo "$(command -v portcheck)" list -l
```

## Uninstall

Run `portcheck browser uninstall` first if you connected browsers. Then remove the
binary (the one-command installer put it in `~/.local/bin` or
`%LOCALAPPDATA%\Programs\portcheck`), or use your package manager
(`brew uninstall portcheck`, `scoop uninstall portcheck`, `apt remove portcheck`, …).
portcheck writes no other configuration or cache files.
