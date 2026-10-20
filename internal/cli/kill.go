package cli

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

type killFlags struct {
	pids        []int32
	protos      []string
	yes         bool
	force       bool
	grace       time.Duration
	dryRun      bool
	allowSystem bool
}

// killTarget is a process to stop and the ports it was found on.
type killTarget struct {
	pid      int32
	name     string
	ports    []uint16
	ancestry string // "npm (4200) ← zsh (900)", if known
}

func (c *cli) newKillCmd() *cobra.Command {
	var f killFlags
	cmd := &cobra.Command{
		Use:   "kill [port...]",
		Short: "Stop the process listening on a port",
		Long: `Stop the process(es) listening on the given ports, or given PIDs.

portcheck first asks the process to exit (SIGTERM) and waits for --grace
before force-killing it. On Windows both steps use TerminateProcess.

You are asked for confirmation unless --yes is given; without a terminal,
--yes is required. portcheck never kills PID 0/1, itself or its parent shell,
and refuses well-known system processes unless --allow-system is given.`,
		Example: `  portcheck kill 3000
  portcheck kill 3000 3001 --yes
  portcheck kill --pid 4242 --force
  portcheck kill 8080 --dry-run`,
		Args: usageArgs(cobra.ArbitraryArgs),
		RunE: func(cmd *cobra.Command, args []string) error { return c.runKill(cmd, args, f) },
	}
	fl := cmd.Flags()
	fl.Int32SliceVar(&f.pids, "pid", nil, "kill these PIDs (in addition to port owners)")
	fl.StringSliceVar(&f.protos, "proto", []string{"tcp", "udp"}, "protocols to match ports on")
	fl.BoolVarP(&f.yes, "yes", "y", false, "do not ask for confirmation")
	fl.BoolVarP(&f.force, "force", "f", false, "kill immediately without a graceful request")
	fl.DurationVar(&f.grace, "grace", killer.DefaultGrace, "how long to wait for a graceful exit")
	fl.BoolVar(&f.dryRun, "dry-run", false, "show what would be killed without doing it")
	fl.BoolVar(&f.allowSystem, "allow-system", false, "allow killing protected system processes (dangerous)")
	return cmd
}

func (c *cli) runKill(cmd *cobra.Command, args []string, f killFlags) error {
	if len(args) == 0 && len(f.pids) == 0 {
		return usageError("specify at least one port or --pid")
	}
	if f.grace <= 0 {
		return usageError("--grace must be positive")
	}
	protos, err := parseProtos(f.protos)
	if err != nil {
		return err
	}
	ctx := cmd.Context()

	targets, err := c.killTargets(cmd, args, protos, f.pids)
	if err != nil {
		return err
	}
	// Run safety guards up front so refusals are shown before confirmation.
	var results []output.KillResultJSON
	var allowed []killTarget
	worst := ExitOK
	for _, t := range targets {
		name, err := c.app.Killer.Check(ctx, t.pid, f.allowSystem)
		if err != nil {
			results = append(results, failed(t.pid, cmp.Or(name, t.name), err))
			worst = max(worst, killExitCode(err))
			continue
		}
		t.name = name
		allowed = append(allowed, t)
	}

	if len(allowed) > 0 && !f.yes && !f.dryRun {
		ok, err := c.confirm(allowed, f.force)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(c.app.Stderr, "Aborted.")
			return negative()
		}
	}

	opts := killer.Options{Grace: f.grace, Force: f.force, DryRun: f.dryRun, AllowProtected: f.allowSystem}
	for _, t := range allowed {
		res, err := c.app.Killer.Kill(ctx, t.pid, opts)
		if err != nil {
			results = append(results, failed(t.pid, cmp.Or(res.Name, t.name), err))
			worst = max(worst, killExitCode(err))
			continue
		}
		results = append(results, output.KillResultJSON{PID: res.PID, Name: res.Name, Outcome: res.Outcome.String()})
	}
	if err := c.printer.Print(output.KillDoc(results)); err != nil {
		return err
	}
	if worst == ExitPermission {
		c.hintPrivileges()
	}
	if worst != ExitOK {
		return &ExitError{Code: worst, Silent: true}
	}
	return nil
}

// killTargets resolves ports to owning processes and merges explicit PIDs.
// It never returns an empty list without an error: when nothing listens on the
// given ports it explains why on stderr and returns a negative result.
func (c *cli) killTargets(cmd *cobra.Command, args []string, protos []inspect.Proto, pids []int32) ([]killTarget, error) {
	byPID := map[int32]*killTarget{}
	for _, pid := range pids {
		byPID[pid] = &killTarget{pid: pid}
	}
	if len(args) > 0 {
		spec, err := portspec.Parse(strings.Join(args, ","))
		if err != nil {
			return nil, &ExitError{Code: ExitUsage, Err: err}
		}
		res, err := c.app.Lister.List(cmd.Context(), inspect.Filter{Ports: spec, Protos: protos, ListeningOnly: true})
		if err != nil {
			return nil, err
		}
		hidden := false
		published := map[inspect.Container][]uint16{}
		for _, s := range res.Sockets {
			if s.Container != nil {
				// Killing the proxy (docker-proxy, com.docker.backend, …) would
				// break every container; point the user at the container instead.
				if ports := published[*s.Container]; !slices.Contains(ports, s.Port()) {
					published[*s.Container] = append(ports, s.Port())
				}
				continue
			}
			if s.PID <= 0 {
				hidden = true
				continue
			}
			t := byPID[s.PID]
			if t == nil {
				t = &killTarget{pid: s.PID}
				byPID[s.PID] = t
			}
			if s.Process != nil {
				t.name = s.Process.Name
				t.ancestry = output.Ancestry(s.Process)
			}
			if !slices.Contains(t.ports, s.Port()) {
				t.ports = append(t.ports, s.Port())
			}
		}
		c.explainContainers(published)
		if len(byPID) == 0 {
			if len(published) > 0 {
				return nil, negative()
			}
			if hidden {
				c.hintPrivileges()
				return nil, &ExitError{Code: ExitPermission, Err: fmt.Errorf("port %s is in use but its owner is not visible", spec)}
			}
			fmt.Fprintf(c.app.Stderr, "Nothing is listening on port %s.\n", spec)
			return nil, negative()
		}
	}
	out := make([]killTarget, 0, len(byPID))
	for _, pid := range slices.Sorted(maps.Keys(byPID)) {
		out = append(out, *byPID[pid])
	}
	return out, nil
}

// explainContainers tells the user how to stop containers publishing ports.
func (c *cli) explainContainers(published map[inspect.Container][]uint16) {
	ctrs := slices.SortedFunc(maps.Keys(published), func(a, b inspect.Container) int {
		return cmp.Compare(output.ContainerLabel(&a), output.ContainerLabel(&b))
	})
	for _, ctr := range ctrs {
		ps := make([]string, len(published[ctr]))
		for i, p := range published[ctr] {
			ps[i] = strconv.Itoa(int(p))
		}
		fmt.Fprintf(c.app.Stderr, "Port %s is %s. Stop it there instead:\n  %s\n",
			strings.Join(ps, ", "), output.DescribeContainer(&ctr), output.StopHint(&ctr))
	}
}

func (c *cli) confirm(targets []killTarget, force bool) (bool, error) {
	if !c.app.StdinTTY {
		return false, usageError("refusing to kill without confirmation in a non-interactive session; pass --yes")
	}
	verb := "terminate"
	if force {
		verb = "force-kill"
	}
	fmt.Fprintf(c.app.Stderr, "About to %s:\n", verb)
	for _, t := range targets {
		line := fmt.Sprintf("  PID %d  %s", t.pid, cmp.Or(t.name, "?"))
		if len(t.ports) > 0 {
			ps := make([]string, len(t.ports))
			for i, p := range t.ports {
				ps[i] = strconv.Itoa(int(p))
			}
			line += "  (port " + strings.Join(ps, ", ") + ")"
		}
		if t.ancestry != "" {
			line += ", started by " + t.ancestry
		}
		fmt.Fprintln(c.app.Stderr, line)
	}
	fmt.Fprint(c.app.Stderr, "Proceed? [y/N] ")
	answer, err := bufio.NewReader(c.app.Stdin).ReadString('\n')
	if err != nil && answer == "" {
		return false, nil // EOF counts as "no"
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func failed(pid int32, name string, err error) output.KillResultJSON {
	return output.KillResultJSON{PID: pid, Name: name, Outcome: "failed", Error: err.Error()}
}

func killExitCode(err error) int {
	switch {
	case isPermission(err):
		return ExitPermission
	case errors.Is(err, killer.ErrProtected), errors.Is(err, killer.ErrNotFound):
		return ExitNegative
	default:
		return ExitRuntime
	}
}
