# Install

portcheck is a single static binary with no runtime dependencies.

=== "Homebrew (macOS, Linux)"

    ```sh
    brew install roystondwork-dev/tap/portcheck
    ```

=== "Scoop (Windows)"

    ```powershell
    scoop bucket add portcheck https://github.com/roystondwork-dev/scoop-bucket
    scoop install portcheck
    ```

=== "Linux packages"

    Download the `.deb`, `.rpm` or `.apk` for your architecture from the
    [latest release](https://github.com/roystondwork-dev/portcheck/releases/latest), then:

    ```sh
    sudo apt install ./portcheck_*_linux_amd64.deb      # Debian, Ubuntu
    sudo dnf install ./portcheck_*_linux_amd64.rpm      # Fedora, RHEL
    sudo apk add --allow-untrusted ./portcheck_*.apk    # Alpine
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

!!! note "Before the first release"
    Until `v0.1.0` is published, only the **Go** method is available.

Check the installation:

```sh
portcheck version
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
      --certificate-identity-regexp '^https://github.com/roystondwork-dev/portcheck/\.github/workflows/release\.yml@refs/tags/v' \
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

Remove the binary, or use your package manager (`brew uninstall portcheck`,
`scoop uninstall portcheck`, `apt remove portcheck`, …). portcheck writes no
configuration or cache files.
