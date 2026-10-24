# AI assistants (MCP)

`portcheck mcp` runs portcheck as a [Model Context Protocol](https://modelcontextprotocol.io)
server, so AI coding assistants can see what's using a port and fix it themselves. When
`npm run dev` fails with `EADDRINUSE` in an agent session, the assistant checks which
process holds the port, tells you who started it, and stops it if you agree, or picks a
free port instead.

```text
You:        npm run dev fails, port 3000 is in use. Fix it.
Assistant:  [check_port 3000] Port 3000 is held by node (pid 41852), started by
            npm ← zsh 2 hours ago: an older dev server. Stop it?
You:        yes
Assistant:  [stop_process port 3000] Stopped node (pid 41852). [npm run dev] Running
            on http://localhost:3000.
```

## Tools

| Tool | What it does |
|---|---|
| `check_port` | Whether ports are free; for each port in use, the process, PID, user, command, who started it, or the container |
| `list_ports` | Every listening port, filterable by port range, process name or protocol |
| `find_free_port` | Free ports, optionally in a range |
| `wait_for_port` | Waits until a server accepts connections (or, with `free`, until a port is free) |
| `stop_process` | Stops what listens on a port, or a PID; `dry_run` reports what it would stop |

Each tool returns the same [JSON](../json-schema.md) as the matching `--json` command.
`stop_process` is marked destructive, so assistants ask before using it, and it keeps
[`kill`'s safety guards](kill.md#safety): it never stops system processes or the
assistant itself, and ports published by Docker, Podman or WSL get the right stop
command instead.

## Set up

[Install portcheck](../install.md) first, then add it to your assistant. If your
assistant can't find `portcheck`, use its full path (`which portcheck`), or run it
through npm: `"command": "npx", "args": ["-y", "port-check", "mcp"]`.

=== "Claude Code"

    ```sh
    claude mcp add --scope user portcheck -- portcheck mcp
    ```

=== "VS Code (Copilot)"

    ```sh
    code --add-mcp '{"name":"portcheck","command":"portcheck","args":["mcp"]}'
    ```

    or in `.vscode/mcp.json`:

    ```json
    { "servers": { "portcheck": { "type": "stdio", "command": "portcheck", "args": ["mcp"] } } }
    ```

=== "Cursor"

    In `~/.cursor/mcp.json` (or `.cursor/mcp.json` in a project):

    ```json
    { "mcpServers": { "portcheck": { "command": "portcheck", "args": ["mcp"] } } }
    ```

=== "Windsurf"

    In `~/.codeium/windsurf/mcp_config.json`:

    ```json
    { "mcpServers": { "portcheck": { "command": "portcheck", "args": ["mcp"] } } }
    ```

=== "Claude Desktop"

    In **Settings → Developer → Edit Config** (`claude_desktop_config.json`), with the
    full path, since desktop apps don't see your shell's `PATH`:

    ```json
    { "mcpServers": { "portcheck": { "command": "/opt/homebrew/bin/portcheck", "args": ["mcp"] } } }
    ```

=== "Zed"

    Install the **Portcheck** extension (**Extensions → search "portcheck"**). Or, in
    the Agent Panel's settings, **Add Custom Server** with the command
    `portcheck mcp`.

=== "Codex CLI"

    In `~/.codex/config.toml`:

    ```toml
    [mcp_servers.portcheck]
    command = "portcheck"
    args = ["mcp"]
    ```

Other MCP clients work the same way: a stdio server started with `portcheck mcp`.
