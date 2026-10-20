package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
	"github.com/roystondwork-dev/portcheck/pkg/scan"
)

const authorizationNote = "note: only scan hosts and networks you own or are authorized to test."

type scanFlags struct {
	ports       string
	concurrency int
	timeout     time.Duration
	rate        float64
	retries     int
	maxHosts    int
	openOnly    bool
}

func (c *cli) newScanCmd() *cobra.Command {
	var f scanFlags
	cmd := &cobra.Command{
		Use:   "scan <host|ip|cidr>...",
		Short: "Check which TCP ports are reachable on remote hosts",
		Long: `Check which TCP ports accept connections on remote hosts.

portcheck makes ordinary TCP connections, so it needs no special privileges.
It has no stealth or evasion features. Each port is reported as:

  open      a connection was established
  closed    the host refused the connection
  filtered  no answer before --timeout (often a firewall)
  error     another failure, e.g. no route to host

Only scan hosts and networks you own or are authorized to test.

Exit status is 0 if any port is open and 1 if none are.`,
		Example: `  portcheck scan db.internal -p 5432
  portcheck scan 10.0.0.0/28 -p 22,80,443 --open
  portcheck scan example.com -p well-known --rate 100 --json`,
		Args: usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error { return c.runScan(cmd, args, f) },
	}
	fl := cmd.Flags()
	fl.StringVarP(&f.ports, "ports", "p", "common", "ports to scan, e.g. 22,80,8000-8100 or a preset (common, well-known, ...)")
	fl.IntVar(&f.concurrency, "concurrency", scan.DefaultConcurrency, "simultaneous connection attempts")
	fl.DurationVar(&f.timeout, "timeout", scan.DefaultTimeout, "per-connection timeout")
	fl.Float64Var(&f.rate, "rate", scan.DefaultRate, "maximum new connections per second (0 = unlimited)")
	fl.IntVar(&f.retries, "retries", 0, "extra attempts for ports that time out")
	fl.IntVar(&f.maxHosts, "max-hosts", scan.DefaultMaxHosts, "refuse to scan more hosts than this")
	fl.BoolVar(&f.openOnly, "open", false, "only show open ports")
	return cmd
}

func (f scanFlags) validate() error {
	switch {
	case f.concurrency < 1:
		return usageError("--concurrency must be at least 1")
	case f.timeout <= 0:
		return usageError("--timeout must be positive")
	case f.rate < 0:
		return usageError("--rate must not be negative")
	case f.retries < 0:
		return usageError("--retries must not be negative")
	case f.maxHosts < 1:
		return usageError("--max-hosts must be at least 1")
	}
	return nil
}

func (c *cli) runScan(cmd *cobra.Command, args []string, f scanFlags) error {
	if err := f.validate(); err != nil {
		return err
	}
	ports, err := portspec.Parse(f.ports)
	if err != nil {
		return &ExitError{Code: ExitUsage, Err: err}
	}
	ctx := cmd.Context()
	targets, err := scan.Expand(ctx, c.app.Resolver, args, f.maxHosts)
	switch {
	case errors.Is(err, scan.ErrBadTarget), errors.Is(err, scan.ErrTooManyHosts):
		return &ExitError{Code: ExitUsage, Err: err}
	case err != nil:
		return err
	}
	if slices.ContainsFunc(targets, func(t scan.Target) bool { return isPublic(t.Addr) }) {
		fmt.Fprintln(c.app.Stderr, authorizationNote)
	}

	rate := f.rate
	if rate == 0 {
		rate = -1 // unlimited
	}
	opts := scan.Options{Concurrency: f.concurrency, Timeout: f.timeout, Rate: rate, Retries: f.retries}
	sum := scan.Summary{Hosts: len(targets), Ports: ports.Len()}
	total := sum.Hosts * sum.Ports
	c.log.Debug("starting scan", "hosts", sum.Hosts, "ports", sum.Ports, "probes", total)

	start := time.Now()
	progress := c.newProgress(total)
	var results []scan.Result
	for r := range c.app.Scan(ctx, targets, ports, opts) {
		sum.Add(r)
		progress.update(sum)
		if !f.openOnly || r.State == scan.Open {
			results = append(results, r)
		}
	}
	progress.done()
	took := time.Since(start)

	order := make(map[netip.Addr]int, len(targets))
	for i, t := range targets {
		order[t.Addr] = i
	}
	slices.SortFunc(results, func(a, b scan.Result) int {
		return cmp.Or(cmp.Compare(order[a.Target.Addr], order[b.Target.Addr]), cmp.Compare(a.Port, b.Port))
	})

	if err := c.printer.Print(output.ScanDoc(results, sum, took)); err != nil {
		return err
	}
	fmt.Fprintf(c.app.Stderr, "Scanned %d host(s) × %d port(s) in %s: %d open, %d closed, %d filtered, %d errors.\n",
		sum.Hosts, sum.Ports, roundDuration(took), sum.Open, sum.Closed, sum.Filtered, sum.Errored)

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("scan interrupted: %w", context.Cause(ctx))
	}
	if sum.Open == 0 {
		return negative()
	}
	return nil
}

// roundDuration keeps sub-millisecond durations readable instead of "0s".
func roundDuration(d time.Duration) time.Duration {
	if d < time.Millisecond {
		return d.Round(time.Microsecond)
	}
	return d.Round(time.Millisecond)
}

// isPublic reports whether a is a globally routable address.
func isPublic(a netip.Addr) bool {
	return !a.IsLoopback() && !a.IsPrivate() && !a.IsLinkLocalUnicast() && !a.IsUnspecified()
}

// progress shows a live counter on stderr when it is a terminal.
type progress struct {
	c     *cli
	total int
	last  time.Time
	shown bool
}

func (c *cli) newProgress(total int) *progress { return &progress{c: c, total: total} }

func (p *progress) update(s scan.Summary) {
	if !p.c.app.StderrTTY {
		return
	}
	n := s.Probes()
	if n < p.total && time.Since(p.last) < 100*time.Millisecond {
		return
	}
	p.last, p.shown = time.Now(), true
	fmt.Fprintf(p.c.app.Stderr, "\r\x1b[Kscanning %d/%d (%d open)", n, p.total, s.Open)
}

func (p *progress) done() {
	if p.shown {
		fmt.Fprint(p.c.app.Stderr, "\r\x1b[K")
	}
}
