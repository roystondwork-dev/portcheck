# GitHub Action

The portcheck action installs portcheck in a GitHub Actions job (Linux, macOS or Windows)
and can wait for services to come up, which is what most CI pipelines need before running
integration tests.

## Wait for services

```yaml
services:
  postgres:
    image: postgres:17
    env: { POSTGRES_PASSWORD: postgres }
    ports: ["5432:5432"]
steps:
  - uses: actions/checkout@v4
  - run: docker compose up -d redis           # or start your app in the background
  - uses: roystondwork-dev/portcheck@v0.12.0
    with:
      wait: localhost:5432 localhost:6379
      timeout: 2m
  - run: npm test
```

The step succeeds as soon as **every** target accepts a TCP connection, and fails when
`timeout` passes first, with portcheck's report of which target never came up. A target is
a port on localhost, or `host:port`.

## Run any portcheck command

`args` runs any other portcheck command line after `wait`, and its standard output is the
step's `output`:

```yaml
- id: port
  uses: roystondwork-dev/portcheck@v0.12.0
  with:
    args: free 3000-3999
- run: PORT=${{ steps.port.outputs.output }} npm start &
```

With neither `wait` nor `args`, the action only installs portcheck, and later steps can run
`portcheck` themselves:

```yaml
- uses: roystondwork-dev/portcheck@v0.12.0
- run: portcheck wait 8080 --timeout 90s && portcheck list -l
```

## Inputs

| Input | Default | |
|---|---|---|
| `wait` | | ports or `host:port` targets to wait for, separated by spaces |
| `timeout` | `60s` | how long `wait` waits, e.g. `30s`, `2m` |
| `args` | | any portcheck command line to run after `wait` |
| `version` | `latest` | the portcheck release to install, e.g. `0.11.1` |

The download is checked against the release's published checksums, the same way the
[one-line installers](../install.md) check it.

Pin the action to a release tag (`@v0.12.0`) or a commit SHA, as for any action.
