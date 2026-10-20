package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/internal/project"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
	"github.com/roystondwork-dev/portcheck/pkg/waitfor"
)

// doctorDialTimeout bounds the connection attempt for expect: open entries.
const doctorDialTimeout = 2 * time.Second

func (c *cli) newDoctorCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check a project's ports against .portcheck.yaml",
		Long: `Check the ports listed in the project's .portcheck.yaml, which is found by
walking up from the current directory, and explain how to fix any problem.

Each entry expects its port to be free (nothing may listen, so the project can
use it) or open (a service such as a database must accept connections).

Exit status is 0 when every check passes, 1 when any fails, and 2 when the
configuration is missing or invalid.`,
		Example: `  portcheck doctor
  portcheck doctor --json
  portcheck doctor --config ./deploy/.portcheck.yaml`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error { return c.runDoctor(cmd, configPath) },
	}
	cmd.Flags().StringVar(&configPath, "config", "", "config file (default: nearest .portcheck.yaml)")
	return cmd
}

// loadProject finds (unless path is set) and loads .portcheck.yaml. Missing or
// invalid configuration is a usage error.
func (c *cli) loadProject(path string) (*project.Config, error) {
	if path == "" {
		wd, err := c.app.Workdir()
		if err != nil {
			return nil, fmt.Errorf("working directory: %w", err)
		}
		if path, err = project.Find(wd); err != nil {
			return nil, &ExitError{Code: ExitUsage, Err: fmt.Errorf("%w; create one, see https://roystondwork-dev.github.io/portcheck/guide/doctor/", err)}
		}
	}
	cfg, err := project.Load(path)
	if err != nil {
		return nil, &ExitError{Code: ExitUsage, Err: err}
	}
	return cfg, nil
}

func (c *cli) runDoctor(cmd *cobra.Command, path string) error {
	cfg, err := c.loadProject(path)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	checks := make([]output.DoctorCheckJSON, len(cfg.Ports))
	var wg sync.WaitGroup
	for i, p := range cfg.Ports {
		wg.Go(func() { checks[i] = c.doctorCheck(ctx, p) })
	}
	wg.Wait()

	if err := c.printer.Print(output.DoctorDoc(cfg.Path, checks)); err != nil {
		return err
	}
	failed := 0
	for _, ch := range checks {
		if !ch.OK {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(c.app.Stderr, "%d of %d checks need attention (%s).\n", failed, len(checks), cfg.Path)
		return negative()
	}
	fmt.Fprintf(c.app.Stderr, "All %d checks passed (%s).\n", len(checks), cfg.Path)
	return nil
}

func (c *cli) doctorCheck(ctx context.Context, p project.Port) output.DoctorCheckJSON {
	res := output.DoctorCheckJSON{
		Name: p.Name, Port: uint16(p.Port), Proto: p.Proto, Expect: p.Expect, Host: p.Host, Description: p.Description,
	}
	if p.Expect == project.ExpectOpen {
		addr := net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
		if waitfor.DialCheck(c.app.Dialer, "tcp", addr, doctorDialTimeout)(ctx) {
			res.OK, res.Status = true, "ok"
			return res
		}
		res.Status = "closed"
		res.Detail = "nothing accepts connections on " + addr
		res.Fix = "start " + p.Name
		if p.Description != "" {
			res.Fix += " (" + p.Description + ")"
		}
		return res
	}

	sts, err := c.app.Ports.CheckAll(ctx, portspec.Single(uint16(p.Port)), inspect.Proto(p.Proto))
	if err != nil {
		res.Status, res.Detail = "error", err.Error()
		return res
	}
	st := sts[0]
	switch st.Availability {
	case freeport.Free:
		res.OK, res.Status = true, "ok"
	case freeport.Denied:
		res.Status = "denied"
		res.Detail = "your user can't bind this port"
		res.Fix = "use a port ≥ 1024 or run with elevated privileges"
	default:
		res.Status = "in-use"
		res.Detail, res.Fix = holderAdvice(st, p.Port)
	}
	return res
}

// holderAdvice describes who holds a port and how to free it.
func holderAdvice(st freeport.Status, port int) (detail, fix string) {
	detail = describeBusy(st)
	killFix := "portcheck kill " + strconv.Itoa(port)
	for _, h := range st.Holders {
		switch {
		case h.Container != nil:
			return detail, output.StopHint(h.Container)
		case h.Process != nil:
			if a := output.Ancestry(h.Process); a != "" {
				detail += ", started by " + a
			}
			return detail, killFix
		}
	}
	// Owner hidden from this user.
	if runtime.GOOS == "windows" {
		return detail, killFix + "  (from an Administrator terminal)"
	}
	return detail, "sudo " + killFix
}

// errNoProjectPort is returned when a port name isn't in .portcheck.yaml.
var errNoProjectPort = errors.New("unknown port name")
