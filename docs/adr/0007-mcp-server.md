# ADR-0007: An MCP server for AI coding assistants

- **Status:** Accepted
- **Date:** 2026-10-24

## Context

AI coding assistants (Claude Code, Cursor, Copilot, Windsurf, Codex) start dev servers
for their users and hit "port already in use" like everyone else. Without help they
guess: run `lsof`, parse it, `kill -9` whatever they find. The Model Context Protocol
(MCP) lets a program offer them typed tools instead.

## Decision

1. **`portcheck mcp`** serves MCP on stdin/stdout, with the official Go SDK
   (`github.com/modelcontextprotocol/go-sdk`).
2. **Five tools, mirroring commands:** `list_ports`, `check_port`, `find_free_port`,
   `wait_for_port` and `stop_process`. Like the browser host
   ([ADR-0005](0005-browser-extension.md)), each runs the command in-process with
   `--json` and returns its schema v1 output with the exit code explained, so
   assistants see what the CLI prints. Inputs are validated into a fixed set of
   arguments; `--allow-system` is never passed.
3. **Safety:** `stop_process` is annotated destructive (assistants ask the user first)
   and keeps `kill`'s guards. The assistant is portcheck's parent process, so it can't
   be stopped.

## Consequences

- The SDK adds dependencies (JSON schema, OAuth for HTTP transports we don't use);
  govulncheck covers them.
- New tools must stay thin wrappers over commands, so the CLI remains the single source
  of behaviour.
