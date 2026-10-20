package cli

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
	"github.com/roystondwork-dev/portcheck/pkg/waitfor"
)

const (
	defaultWaitTimeout  = 30 * time.Second
	defaultWaitInterval = 250 * time.Millisecond
	minWaitInterval     = 10 * time.Millisecond
	// waitDialTimeout bounds one connection attempt, so a host that silently
	// drops packets is retried instead of consuming the whole wait.
	waitDialTimeout = 2 * time.Second
)

type waitFlags struct {
	host     string
	timeout  time.Duration
	interval time.Duration
	free     bool
}

type waitTarget struct {
	arg  string
	host string
	port uint16
}

func (c *cli) newWaitCmd() *cobra.Command {
	var f waitFlags
	cmd := &cobra.Command{
		Use:   "wait <port|host:port>...",
		Short: "Wait until ports accept connections (or become free)",
		Long: `Wait until every target accepts TCP connections, or with --free until
every local port is free.

A target is a port (on --host, default localhost) or host:port. All targets are
awaited in parallel. Exit status is 0 when all are ready, 1 on timeout.`,
		Example: `  portcheck wait 5432                          # until Postgres accepts connections
  portcheck wait db:5432 redis:6379 -t 60s     # several services, up to a minute
  portcheck wait 3000 --free                   # until the old dev server is gone
  portcheck wait 8080 -t 0                     # wait forever`,
		Args: usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error { return c.runWait(cmd, args, f) },
	}
	fl := cmd.Flags()
	fl.StringVar(&f.host, "host", "localhost", "host for targets given as a bare port")
	fl.DurationVarP(&f.timeout, "timeout", "t", defaultWaitTimeout, "give up after this long (0 = wait forever)")
	fl.DurationVar(&f.interval, "interval", defaultWaitInterval, "time between attempts")
	fl.BoolVar(&f.free, "free", false, "wait until local ports are free instead of open")
	return cmd
}

func parseWaitTargets(args []string, f waitFlags) ([]waitTarget, error) {
	targets := make([]waitTarget, 0, len(args))
	for _, arg := range args {
		host, portStr := f.host, arg
		if strings.Contains(arg, ":") {
			if f.free {
				return nil, usageError("--free only works with local ports; drop the host from %q", arg)
			}
			var err error
			if host, portStr, err = net.SplitHostPort(arg); err != nil || host == "" {
				return nil, usageError("invalid target %q (want PORT or HOST:PORT)", arg)
			}
		}
		port, err := portspec.ParsePort(portStr)
		if err != nil {
			return nil, usageError("invalid port in %q: %v", arg, err)
		}
		targets = append(targets, waitTarget{arg: arg, host: host, port: port})
	}
	return targets, nil
}

func (c *cli) runWait(cmd *cobra.Command, args []string, f waitFlags) error {
	switch {
	case f.timeout < 0:
		return usageError("--timeout must not be negative")
	case f.interval < minWaitInterval:
		return usageError("--interval must be at least %s", minWaitInterval)
	case f.free && f.host != "localhost":
		return usageError("--free only works with local ports; drop --host")
	}
	targets, err := parseWaitTargets(args, f)
	if err != nil {
		return err
	}

	parent := cmd.Context()
	ctx := parent
	if f.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, f.timeout)
		defer cancel()
	}

	condition, verb := "open", "accept connections"
	if f.free {
		condition, verb = "free", "be free"
	}
	limit := "no timeout"
	if f.timeout > 0 {
		limit = "timeout " + f.timeout.String()
	}
	fmt.Fprintf(c.app.Stderr, "Waiting for %s to %s (%s)…\n", describeTargets(targets), verb, limit)

	results := make([]output.WaitResultJSON, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Go(func() {
			r := waitfor.Poll(ctx, f.interval, c.waitCheck(t, f.free))
			c.log.Debug("wait finished", "target", t.arg, "ready", r.Ready, "attempts", r.Attempts, "waited", r.Waited)
			results[i] = output.WaitResultJSON{
				Target: t.arg, Host: t.host, Port: t.port, Condition: condition,
				Ready: r.Ready, WaitedMs: r.Waited.Milliseconds(), Attempts: r.Attempts,
			}
		})
	}
	wg.Wait()

	if err := c.printer.Print(output.WaitDoc(results)); err != nil {
		return err
	}
	if err := parent.Err(); err != nil {
		return fmt.Errorf("wait interrupted: %w", err)
	}
	for _, r := range results {
		if !r.Ready {
			return negative()
		}
	}
	return nil
}

func (c *cli) waitCheck(t waitTarget, free bool) waitfor.Check {
	if free {
		return func(ctx context.Context) bool {
			sts, err := c.app.Ports.CheckAll(ctx, portspec.Single(t.port), inspect.TCP)
			return err == nil && sts[0].Free()
		}
	}
	addr := net.JoinHostPort(t.host, strconv.Itoa(int(t.port)))
	return waitfor.DialCheck(c.app.Dialer, "tcp", addr, waitDialTimeout)
}

func describeTargets(ts []waitTarget) string {
	if len(ts) == 1 {
		return net.JoinHostPort(ts[0].host, strconv.Itoa(int(ts[0].port)))
	}
	return strconv.Itoa(len(ts)) + " targets"
}
