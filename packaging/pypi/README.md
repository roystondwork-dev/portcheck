# portcheck

See what's using a port and free it safely: which process, who started it, stop it.
Fixes "port already in use" (`EADDRINUSE`, `Address already in use`).

```sh
pipx install port-check     # or: pip install port-check / uv tool install port-check

portcheck 3000              # who holds port 3000? (exit 1 if in use)
portcheck kill 3000         # stop it, after confirming; never system processes
portcheck free              # a free port
portcheck                   # interactive view of every listening port
```

```text
PORT  PROTO  STATUS  PID    PROCESS  USER
3000  tcp    in-use  41852  node     alice
```

Works on macOS, Linux and Windows (x64 and arm64). This package installs the native
`portcheck` binary; there's no Python code and nothing is downloaded at runtime.

Also available for VS Code, JetBrains IDEs, Edge and Firefox, and through Homebrew,
Scoop and npm (`npx port-check 3000`).

- Documentation: https://roystondwork-dev.github.io/portcheck/
- Source (MIT): https://github.com/roystondwork-dev/portcheck
