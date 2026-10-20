package cli

import (
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/tui"
)

const defaultTUIInterval = 2 * time.Second

func (c *cli) newTUICmd() *cobra.Command {
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Live, interactive view of ports (the default in a terminal)",
		Long: `Open a live, interactive view of local sockets and their processes.

Move with the arrow keys, filter with /, switch protocols with tab, toggle
listening-only with l, sort with s/S, stop the selected process with x
(K to force-kill, after confirmation), and press ? for all keys.`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error { return c.runTUI(cmd, interval) },
	}
	cmd.Flags().DurationVar(&interval, "interval", defaultTUIInterval, "refresh interval")
	return cmd
}

func (c *cli) runTUI(cmd *cobra.Command, interval time.Duration) error {
	if interval <= 0 {
		return usageError("--interval must be positive")
	}
	if !c.app.StdinTTY || !c.app.StdoutTTY {
		return usageError("the TUI needs an interactive terminal; use 'portcheck list' instead")
	}
	deps := tui.Deps{Lister: c.app.Lister, Killer: c.app.Killer, Interval: interval, Hint: c.app.PrivilegeHint()}
	environ := c.app.Environ
	if c.noColor {
		// The TUI detects color support from the environment.
		environ = append(slices.Clone(environ), "NO_COLOR=1")
	}
	return c.app.RunTUI(cmd.Context(), deps, c.app.Stdin, c.app.Stdout, environ)
}
