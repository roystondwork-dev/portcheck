package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

type listFlags struct {
	listening bool
	protos    []string
	ports     string
	pids      []int32
	name      string
	states    []string
	watch     time.Duration
}

func (c *cli) newListCmd() *cobra.Command {
	var f listFlags
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List sockets and the processes that own them",
		Long: `List local TCP and UDP sockets together with their owning process.

Without elevated privileges some owners may be hidden; portcheck prints a hint
when that happens.`,
		Example: `  portcheck list --listening
  portcheck list --port 3000-3999 --proto tcp
  portcheck list --name 'node*' --json
  portcheck list -l --watch 2s`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error { return c.runList(cmd, f) },
	}
	fl := cmd.Flags()
	fl.BoolVarP(&f.listening, "listening", "l", false, "only sockets accepting connections (TCP LISTEN, unconnected UDP)")
	fl.StringSliceVar(&f.protos, "proto", nil, "protocols to include: tcp, udp")
	fl.StringVar(&f.ports, "port", "", "local ports, e.g. 80,443,8000-8100 or a preset (common, well-known, ...)")
	fl.Int32SliceVar(&f.pids, "pid", nil, "only sockets owned by these PIDs")
	fl.StringVar(&f.name, "name", "", "only processes whose name matches this glob (case-insensitive)")
	fl.StringSliceVar(&f.states, "state", nil, "only these states, e.g. LISTEN,ESTABLISHED")
	fl.DurationVar(&f.watch, "watch", 0, "refresh every interval until interrupted, e.g. 2s")
	return cmd
}

func (c *cli) buildListFilter(f listFlags) (inspect.Filter, error) {
	filter := inspect.Filter{ListeningOnly: f.listening, PIDs: f.pids, Name: f.name}
	var err error
	if filter.Protos, err = parseProtos(f.protos); err != nil {
		return filter, err
	}
	if f.ports != "" {
		if filter.Ports, err = portspec.Parse(f.ports); err != nil {
			return filter, &ExitError{Code: ExitUsage, Err: err}
		}
	}
	for _, s := range f.states {
		filter.States = append(filter.States, inspect.State(strings.ToUpper(strings.TrimSpace(s))))
	}
	if err := filter.Validate(); err != nil {
		return filter, &ExitError{Code: ExitUsage, Err: err}
	}
	return filter, nil
}

func (c *cli) runList(cmd *cobra.Command, f listFlags) error {
	filter, err := c.buildListFilter(f)
	if err != nil {
		return err
	}
	if f.watch < 0 {
		return usageError("--watch must be positive")
	}
	ctx := cmd.Context()
	hinted := false
	render := func() error {
		start := time.Now()
		res, err := c.app.Lister.List(ctx, filter)
		if err != nil {
			return err
		}
		c.log.Debug("listed sockets", "count", len(res.Sockets), "partial", res.Partial, "took", time.Since(start))
		if f.watch > 0 && c.app.StdoutTTY {
			_, _ = cmd.OutOrStdout().Write([]byte("\x1b[H\x1b[2J")) // clear screen
		}
		if err := c.printer.Print(output.ListDoc(res)); err != nil {
			return err
		}
		if res.Partial && !hinted {
			hinted = true
			c.hintPrivileges()
		}
		return nil
	}

	if err := render(); err != nil || f.watch == 0 {
		return err
	}
	ticker := time.NewTicker(f.watch)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil // Ctrl-C ends watch mode normally
		case <-ticker.C:
			if err := render(); err != nil {
				return err
			}
		}
	}
}
