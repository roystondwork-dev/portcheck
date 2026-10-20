package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// maxCheckPorts bounds `check` so a typo like "1-65535" does not take minutes.
const maxCheckPorts = 4096

func (c *cli) newCheckCmd() *cobra.Command {
	var proto string
	cmd := &cobra.Command{
		Use:   "check <port>...",
		Short: "Check whether ports are free and show who holds them",
		Long: `Check whether local ports are free. For ports in use, show the owning process.

Exit status is 0 when every port is free and 1 when any port is in use or
cannot be bound, which makes 'check' handy in scripts:

  portcheck check 5432 || echo "postgres port is taken"`,
		Example: `  portcheck check 3000
  portcheck check 3000 8080 5432
  portcheck check 8000-8010 --json
  portcheck 3000                  # shortcut`,
		Args: usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := parseProto(proto)
			if err != nil {
				return err
			}
			return c.runCheck(cmd, args, p)
		},
	}
	cmd.Flags().StringVar(&proto, "proto", "tcp", "protocol: tcp or udp")
	return cmd
}

func (c *cli) runCheck(cmd *cobra.Command, args []string, proto inspect.Proto) error {
	spec, err := portspec.Parse(strings.Join(args, ","))
	if err != nil {
		return &ExitError{Code: ExitUsage, Err: err}
	}
	if spec.Len() > maxCheckPorts {
		return usageError("too many ports (%d); check at most %d at a time", spec.Len(), maxCheckPorts)
	}
	sts, err := c.app.Ports.CheckAll(cmd.Context(), spec, proto)
	if err != nil {
		return err
	}
	if err := c.printer.Print(output.CheckDoc(sts)); err != nil {
		return err
	}
	allFree, hiddenOwner := true, false
	for _, st := range sts {
		if st.Availability != freeport.Free {
			allFree = false
		}
		if st.Availability == freeport.InUse && len(st.Holders) == 0 {
			hiddenOwner = true
		}
	}
	if hiddenOwner {
		c.hintPrivileges()
	}
	if !allFree {
		return negative()
	}
	return nil
}
