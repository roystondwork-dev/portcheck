# portcheck for VS Code

See what's using your ports, and free them safely, without leaving the editor.

- **Listening ports** view: every listening port with its process, who started it
  (`node ← npm ← zsh`), and the Docker/Podman container or WSL distro behind it.
- **Stop** a process from the view or the command palette. You confirm first, and
  portcheck's safety guards apply: it never stops system processes or VS Code itself.
- Container and WSL ports show the right command (`docker stop web`) instead of killing
  the proxy.
- **Check a port** and **find a free port** from the command palette.
- Works in Remote-SSH, WSL and Dev Containers: it runs where your code runs.

The extension uses the [portcheck](https://github.com/roystondwork-dev/portcheck) CLI.
Release builds bundle it; otherwise install portcheck and make sure it's on your `PATH`,
or set **portcheck.path**.

Documentation: <https://roystondwork-dev.github.io/portcheck/guide/vscode/>
