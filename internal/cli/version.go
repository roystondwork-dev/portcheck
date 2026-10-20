package cli

import (
	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
)

func (c *cli) newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(*cobra.Command, []string) error {
			return c.printer.Print(output.VersionDoc(c.app.Build))
		},
	}
}
