// Package scan checks which TCP ports are reachable on remote hosts.
//
// It performs ordinary TCP connections ("connect scan") only: no raw sockets,
// no elevated privileges, and no stealth or evasion techniques (see ADR-0002).
// Concurrency, per-connection timeouts and a global rate limit keep scans
// polite by default.
//
// Only scan hosts and networks you own or are explicitly authorized to test.
package scan

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// State is the outcome of probing one port.
type State string

// Possible states.
const (
	// Open means a TCP connection was established.
	Open State = "open"
	// Closed means the host actively refused the connection.
	Closed State = "closed"
	// Filtered means there was no answer before the timeout (often a firewall).
	Filtered State = "filtered"
	// Error means the probe failed for another reason, e.g. network unreachable.
	Error State = "error"
)

// Result is the outcome for one host and port.
type Result struct {
	Target  Target
	Port    uint16
	State   State
	Service string        // well-known service name, if any
	Latency time.Duration // time of the last connection attempt
	Err     error         // set when State is Error
}

// Dialer opens network connections; *net.Dialer implements it.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Options control a scan. Zero values select the defaults.
type Options struct {
	// Concurrency is the number of simultaneous connection attempts (default 100).
	Concurrency int
	// Timeout is the per-connection timeout (default 1s).
	Timeout time.Duration
	// Rate is the maximum number of new connections per second (default 500).
	// A negative value disables rate limiting.
	Rate float64
	// Retries is how many extra attempts a timed-out ("filtered") port gets (default 0).
	Retries int
	// Dialer overrides how connections are made (default: a net.Dialer).
	Dialer Dialer
}

// Defaults.
const (
	DefaultConcurrency = 100
	DefaultTimeout     = time.Second
	DefaultRate        = 500
)

func (o Options) withDefaults() Options {
	if o.Concurrency < 1 {
		o.Concurrency = DefaultConcurrency
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Rate == 0 {
		o.Rate = DefaultRate
	}
	o.Retries = max(o.Retries, 0)
	if o.Dialer == nil {
		o.Dialer = &net.Dialer{}
	}
	return o
}

func (o Options) limiter() *rate.Limiter {
	if o.Rate < 0 {
		return rate.NewLimiter(rate.Inf, 0)
	}
	return rate.NewLimiter(rate.Limit(o.Rate), o.Concurrency)
}

type job struct {
	target Target
	port   uint16
}

// Scan probes every port in ports on every target and streams results in
// completion order. The channel is closed when the scan finishes or ctx is
// cancelled; probes interrupted by cancellation are not reported.
func Scan(ctx context.Context, targets []Target, ports portspec.Spec, opts Options) <-chan Result {
	opts = opts.withDefaults()
	lim := opts.limiter()
	jobs := make(chan job)
	out := make(chan Result, opts.Concurrency)

	go func() {
		defer close(jobs)
		for _, t := range targets {
			for p := range ports.All() {
				select {
				case jobs <- job{target: t, port: p}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	var wg sync.WaitGroup
	for range opts.Concurrency {
		wg.Go(func() {
			for j := range jobs {
				r, ok := probe(ctx, opts, lim, j)
				if !ok {
					continue
				}
				select {
				case out <- r:
				case <-ctx.Done():
				}
			}
		})
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// probe tries one port, retrying timeouts. ok is false if ctx was cancelled.
func probe(ctx context.Context, opts Options, lim *rate.Limiter, j job) (r Result, ok bool) {
	r = Result{Target: j.target, Port: j.port, Service: Service(j.port)}
	addr := netip.AddrPortFrom(j.target.Addr, j.port).String()
	for attempt := 0; ; attempt++ {
		if err := lim.Wait(ctx); err != nil {
			return r, false
		}
		start := time.Now()
		dctx, cancel := context.WithTimeout(ctx, opts.Timeout)
		conn, err := opts.Dialer.DialContext(dctx, "tcp", addr)
		cancel()
		r.Latency = time.Since(start)
		if err == nil {
			_ = conn.Close()
			r.State, r.Err = Open, nil
			return r, true
		}
		if ctx.Err() != nil {
			return r, false
		}
		r.State, r.Err = classify(err)
		if r.State != Filtered || attempt >= opts.Retries {
			return r, true
		}
	}
}

func classify(err error) (State, error) {
	var ne net.Error
	switch {
	case errors.Is(err, errConnRefused):
		return Closed, nil
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return Filtered, nil
	default:
		return Error, err
	}
}

// Summary tallies scan results.
type Summary struct {
	Hosts, Ports                    int
	Open, Closed, Filtered, Errored int
}

// Add counts r.
func (s *Summary) Add(r Result) {
	switch r.State {
	case Open:
		s.Open++
	case Closed:
		s.Closed++
	case Filtered:
		s.Filtered++
	case Error:
		s.Errored++
	}
}

// Probes returns the number of results counted so far.
func (s Summary) Probes() int { return s.Open + s.Closed + s.Filtered + s.Errored }
