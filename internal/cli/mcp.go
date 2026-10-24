package cli

import (
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/mcpserver"
)

func (c *cli) newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve portcheck to AI assistants (Model Context Protocol)",
		Long: `Run portcheck as an MCP server on stdin/stdout, so AI coding assistants
(Claude Code, Cursor, VS Code Copilot, Windsurf, ...) can check which process
holds a port, find a free port, wait for a server, and stop a process when a
dev server fails with "address already in use".

Tools: list_ports, check_port, find_free_port, wait_for_port, stop_process.
stop_process has the same safety guards as 'portcheck kill': it never stops
system processes or the assistant itself, and assistants ask before using it.

Add it to your assistant (see the MCP guide for others):
  claude mcp add portcheck -- portcheck mcp`,
		Example: `  claude mcp add portcheck -- portcheck mcp
  code --add-mcp '{"name":"portcheck","command":"portcheck","args":["mcp"]}'`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			server := mcpserver.New(c.app.Build.Version, inProcess(c.app))
			t := &mcp.IOTransport{Reader: io.NopCloser(c.app.Stdin), Writer: nopWriteCloser{c.app.Stdout}}
			return server.Run(cmd.Context(), t)
		},
	}
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }
