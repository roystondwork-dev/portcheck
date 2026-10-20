package cli

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
	"github.com/roystondwork-dev/portcheck/pkg/waitfor"
)

const (
	defaultRunRange       = "3000-9999"
	defaultRunWaitTimeout = 60 * time.Second
	portPlaceholder       = "{port}"
)

// runFreeTimeout bounds how long `run --kill` waits for the port to be
// released. A variable so tests can shorten it.
var runFreeTimeout = 10 * time.Second

var envNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type runFlags struct {
	port        int
	portRange   string
	next        bool
	kill        bool
	yes         bool
	force       bool
	grace       time.Duration
	env         string
	wait        bool
	waitTimeout time.Duration
	quiet       bool
}

func (c *cli) newRunCmd() *cobra.Command {
	var f runFlags
	cmd := &cobra.Command{
		Use:   "run [flags] [--] <command> [args...]",
		Short: "Run a command on a free port (sets $PORT)",
		Long: `Run a command on a usable port, passing it in $PORT and replacing {port} in
the command's arguments.

With --port, that port is used if it's free. If it's taken, run explains who
holds it and stops, unless --next picks the next free port or --kill stops the
holder (with the same confirmation and safety guards as 'portcheck kill').
Without --port, the first free port in --range is used.

The command runs in the foreground with your terminal; Ctrl+C reaches it
directly. portcheck exits with the command's exit status. portcheck's own
messages go to stderr.`,
		Example: `  portcheck run --port 3000 -- npm start
  portcheck run --port 3000 --next -- npm run dev
  portcheck run --port 8080 --kill --yes -- ./server
  portcheck run -- python3 -m http.server {port}
  portcheck run --port 5173 --wait -- npx vite --port {port}`,
		Args: usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error { return c.runRun(cmd, args, f) },
	}
	fl := cmd.Flags()
	fl.SetInterspersed(false) // everything after the command name belongs to the command
	fl.IntVarP(&f.port, "port", "p", 0, "port to use (default: first free port in --range)")
	fl.StringVar(&f.portRange, "range", defaultRunRange, "ports to choose from when --port is not given")
	fl.BoolVar(&f.next, "next", false, "if --port is taken, use the next free port above it")
	fl.BoolVar(&f.kill, "kill", false, "if --port is taken, stop the process holding it (asks first)")
	fl.BoolVarP(&f.yes, "yes", "y", false, "with --kill: don't ask for confirmation")
	fl.BoolVarP(&f.force, "force", "f", false, "with --kill: kill immediately without a graceful request")
	fl.DurationVar(&f.grace, "grace", killer.DefaultGrace, "with --kill: how long to wait for a graceful exit")
	fl.StringVar(&f.env, "env", "PORT", "environment variable that receives the port")
	fl.BoolVar(&f.wait, "wait", false, "report when the command starts accepting connections")
	fl.DurationVar(&f.waitTimeout, "wait-timeout", defaultRunWaitTimeout, "with --wait: how long to wait")
	fl.BoolVarP(&f.quiet, "quiet", "q", false, "don't print portcheck's own messages")
	return cmd
}

func (f runFlags) validate() error {
	switch {
	case f.port < 0 || f.port > math.MaxUint16:
		return usageError("--port must be between 1 and 65535")
	case f.next && f.kill:
		return usageError("--next and --kill can't be combined")
	case (f.next || f.kill) && f.port == 0:
		return usageError("--next and --kill need --port")
	case !envNameRE.MatchString(f.env):
		return usageError("--env %q is not a valid environment variable name", f.env)
	case f.grace <= 0:
		return usageError("--grace must be positive")
	case f.waitTimeout <= 0:
		return usageError("--wait-timeout must be positive")
	}
	return nil
}

func (c *cli) runRun(cmd *cobra.Command, args []string, f runFlags) error {
	if err := f.validate(); err != nil {
		return err
	}
	// stdout belongs to the command; portcheck's own output goes to stderr.
	c.printer = output.NewPrinter(c.app.Stderr, c.printer.Format(), output.Options{NoColor: c.noColor, Environ: c.app.Environ})
	say := func(format string, a ...any) {
		if !f.quiet {
			fmt.Fprintf(c.app.Stderr, "portcheck: "+format+"\n", a...)
		}
	}

	port, err := c.choosePort(cmd, f, say)
	if err != nil {
		return err
	}

	spec := RunSpec{
		Name:   args[0],
		Args:   substitutePort(args[1:], port),
		Env:    withEnv(c.app.Environ, f.env, port),
		Stdin:  c.app.Stdin,
		Stdout: c.app.Stdout,
		Stderr: c.app.Stderr,
	}
	say("starting %s with %s=%d", displayCommand(spec.Name, spec.Args), f.env, port)

	waitCtx, stopWaiting := context.WithCancel(cmd.Context())
	var wg sync.WaitGroup
	started := func() {
		if f.wait {
			wg.Go(func() { c.reportReady(waitCtx, port, f.waitTimeout, say) })
		}
	}
	code, err := c.app.Runner.Run(spec, started)
	stopWaiting()
	wg.Wait()

	switch {
	case err != nil && (code == exitNotFound || code == exitCannotExecute):
		return &ExitError{Code: code, Err: fmt.Errorf("cannot run %s: %w", spec.Name, err)}
	case err != nil:
		return fmt.Errorf("run %s: %w", spec.Name, err)
	case code != 0:
		return &ExitError{Code: code, Silent: true}
	}
	return nil
}

// choosePort applies --port/--next/--kill/--range.
func (c *cli) choosePort(cmd *cobra.Command, f runFlags, say func(string, ...any)) (uint16, error) {
	ctx := cmd.Context()
	if f.port == 0 {
		spec, err := portspec.Parse(f.portRange)
		if err != nil {
			return 0, &ExitError{Code: ExitUsage, Err: err}
		}
		return c.firstFree(ctx, spec, say)
	}

	port := uint16(f.port)
	sts, err := c.app.Ports.CheckAll(ctx, portspec.Single(port), inspect.TCP)
	if err != nil {
		return 0, err
	}
	st := sts[0]
	if st.Free() {
		return port, nil
	}
	why := describeBusy(st)

	switch {
	case f.next:
		if port == math.MaxUint16 {
			say("port %d is %s, and there is no higher port", port, why)
			return 0, negative()
		}
		next, err := c.firstFree(ctx, portspec.New(portspec.Range{Start: port + 1, End: math.MaxUint16}), say)
		if err == nil {
			say("port %d is %s; using %d", port, why, next)
		}
		return next, err
	case f.kill:
		if st.Availability == freeport.Denied {
			return 0, &ExitError{Code: ExitPermission, Err: fmt.Errorf("port %d can't be bound by your user, so there is nothing to stop", port)}
		}
		kf := killFlags{protos: []string{"tcp"}, yes: f.yes, force: f.force, grace: f.grace}
		if err := c.runKill(cmd, []string{strconv.Itoa(int(port))}, kf); err != nil {
			return 0, err
		}
		freeCtx, cancel := context.WithTimeout(ctx, runFreeTimeout)
		defer cancel()
		if !waitfor.Poll(freeCtx, 100*time.Millisecond, c.waitCheck(waitTarget{host: "localhost", port: port}, true)).Ready {
			say("port %d is still in use after stopping its holder", port)
			return 0, negative()
		}
		return port, nil
	default:
		say("port %d is %s. Use --next to pick another port, or --kill to stop the holder.", port, why)
		return 0, negative()
	}
}

func (c *cli) firstFree(ctx context.Context, spec portspec.Spec, say func(string, ...any)) (uint16, error) {
	ports, err := c.app.Ports.Find(ctx, spec, inspect.TCP, 1)
	switch {
	case errors.Is(err, freeport.ErrNoFreePort):
		say("no free port in %s", spec)
		return 0, negative()
	case err != nil:
		return 0, err
	}
	return ports[0], nil
}

// describeBusy explains why a port isn't free, e.g. "in use by node (pid 4242)".
func describeBusy(st freeport.Status) string {
	if st.Availability == freeport.Denied {
		return "not usable by your user (permission denied)"
	}
	holders := make([]string, 0, len(st.Holders))
	for _, h := range st.Holders {
		switch {
		case h.Container != nil:
			holders = append(holders, output.ContainerLabel(h.Container))
		case h.Process != nil:
			holders = append(holders, fmt.Sprintf("%s (pid %d)", h.Process.Name, h.PID))
		case h.PID > 0:
			holders = append(holders, fmt.Sprintf("pid %d", h.PID))
		}
	}
	if len(holders) == 0 {
		return "in use"
	}
	return "in use by " + strings.Join(holders, ", ")
}

func (c *cli) reportReady(ctx context.Context, port uint16, timeout time.Duration, say func(string, ...any)) {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	r := waitfor.Poll(tctx, 250*time.Millisecond, c.waitCheck(waitTarget{host: "localhost", port: port}, false))
	switch {
	case r.Ready:
		say("ready on localhost:%d (%s)", port, r.Waited.Round(time.Millisecond))
	case ctx.Err() == nil: // timed out while the command is still running
		say("nothing accepted connections on port %d within %s", port, timeout)
	}
}

// displayCommand renders a command line for messages, quoting arguments
// that contain spaces or quotes so the boundaries are unambiguous.
func displayCommand(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	for _, a := range append([]string{name}, args...) {
		if a == "" || strings.ContainsAny(a, " \t\n\"'") {
			a = strconv.Quote(a)
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

func substitutePort(args []string, port uint16) []string {
	out := make([]string, len(args))
	p := strconv.Itoa(int(port))
	for i, a := range args {
		out[i] = strings.ReplaceAll(a, portPlaceholder, p)
	}
	return out
}

// withEnv returns environ with name=port, replacing any existing value.
func withEnv(environ []string, name string, port uint16) []string {
	out := make([]string, 0, len(environ)+1)
	for _, kv := range environ {
		if k, _, _ := strings.Cut(kv, "="); k != name {
			out = append(out, kv)
		}
	}
	return append(out, name+"="+strconv.Itoa(int(port)))
}
