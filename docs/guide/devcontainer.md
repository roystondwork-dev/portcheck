# Dev containers and Codespaces

The portcheck [Dev Container feature](https://containers.dev/features) installs portcheck
in a dev container: VS Code Dev Containers, GitHub Codespaces, or anything else that
reads `devcontainer.json`. Add it to `.devcontainer/devcontainer.json`:

```json
{
  "image": "mcr.microsoft.com/devcontainers/base:debian",
  "features": {
    "ghcr.io/roystondwork-dev/portcheck/portcheck:1": {}
  }
}
```

Rebuild the container, and `portcheck` is on `PATH` for everyone who opens the project.
Inside the container, it sees the container's own ports: the dev servers you start
there.

## Options

| Option | Default | |
|---|---|---|
| `version` | `latest` | the portcheck release to install, e.g. `0.12.0` |

```json
"features": {
  "ghcr.io/roystondwork-dev/portcheck/portcheck:1": { "version": "0.12.0" }
}
```

The feature runs portcheck's [one-line installer](../install.md), which checks the
download against the release's published checksums. It works on Debian, Ubuntu and
Alpine based images, and installs `curl` first if the image has neither `curl` nor
`wget`.

## A project's ports in the container

Together with a [`.portcheck.yaml`](doctor.md), `portcheck doctor` can check the
container's ports when it starts:

```json
"postStartCommand": "portcheck doctor"
```

For a container image without the feature, see the [Docker image](../install.md) and its
`COPY --from` line.
