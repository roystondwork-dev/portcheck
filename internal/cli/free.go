package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// defaultFreeRange avoids privileged ports, which usually need root.
const defaultFreeRange = "1024-65535"

func (c *cli) newFreeCmd() *cobra.Command {
	var (
		count  int
		random bool
		proto  string
	)
	cmd := &cobra.Command{
		Use:   "free [range]",
		Short: "Find free ports",
		Long: `Find free local ports in a range (default ` + defaultFreeRange + `).

Ports are returned in ascending order, or a random one with --random. Another
process can still grab a port after portcheck reports it, so bind it soon.`,
		Example: `  portcheck free                      # first free port >= 1024
  portcheck free 3000-3999 -n 3       # three free ports in a range
  portcheck free --random 20000-30000
  PORT=$(portcheck free -o plain 8000-9000 | tail -1)`,
		Args: usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := parseProto(proto)
			if err != nil {
				return err
			}
			rangeArg := defaultFreeRange
			if len(args) == 1 {
				rangeArg = args[0]
			}
			spec, err := portspec.Parse(rangeArg)
			if err != nil {
				return &ExitError{Code: ExitUsage, Err: err}
			}
			if count < 1 {
				return usageError("--count must be at least 1")
			}
			if random && count > 1 {
				return usageError("--random returns a single port; drop --count")
			}

			var ports []uint16
			if random {
				var port uint16
				port, err = c.app.Ports.Random(cmd.Context(), spec, p)
				if err == nil {
					ports = []uint16{port}
				}
			} else {
				ports, err = c.app.Ports.Find(cmd.Context(), spec, p, count)
			}
			if err != nil && !errors.Is(err, freeport.ErrNoFreePort) {
				return err
			}
			if perr := c.printer.Print(output.FreeDoc(ports)); perr != nil {
				return perr
			}
			if err != nil {
				return negative()
			}
			return nil
		},
	}
	fl := cmd.Flags()
	fl.IntVarP(&count, "count", "n", 1, "number of free ports to find")
	fl.BoolVar(&random, "random", false, "pick a random free port instead of the lowest")
	fl.StringVar(&proto, "proto", "tcp", "protocol: tcp or udp")
	return cmd
}
