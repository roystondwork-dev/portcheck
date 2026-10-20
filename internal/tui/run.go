package tui

import (
	"context"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
)

// Run starts the TUI on the given terminal streams and blocks until the user
// quits or ctx is cancelled.
func Run(ctx context.Context, deps Deps, in io.Reader, out io.Writer, environ []string) error {
	p := tea.NewProgram(New(ctx, deps),
		tea.WithContext(ctx),
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithEnvironment(environ),
	)
	if _, err := p.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("tui: %w", ctx.Err()) // report Ctrl-C/cancellation, not "program was killed"
		}
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}
