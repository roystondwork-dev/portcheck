package scan

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

type behavior int

const (
	bOpen behavior = iota
	bRefused
	bTimeout    // blocks until the dial context expires
	bNetTimeout // returns a net.Error with Timeout() == true
	bUnreachable
)

// fakeDialer simulates hosts. Ports not listed are refused.
type fakeDialer struct {
	mu        sync.Mutex
	behaviors map[string]behavior // "ip:port"
	calls     map[string]int
	onDial    func()
}

func newFakeDialer(b map[string]behavior) *fakeDialer {
	return &fakeDialer{behaviors: b, calls: map[string]int{}}
}

type netTimeout struct{}

func (netTimeout) Error() string   { return "i/o timeout" }
func (netTimeout) Timeout() bool   { return true }
func (netTimeout) Temporary() bool { return true }

func (d *fakeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d.mu.Lock()
	d.calls[addr]++
	b, ok := d.behaviors[addr]
	onDial := d.onDial
	d.mu.Unlock()
	if network != "tcp" {
		return nil, fmt.Errorf("unexpected network %q", network)
	}
	if onDial != nil {
		onDial()
	}
	if !ok {
		b = bRefused
	}
	switch b {
	case bOpen:
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	case bTimeout:
		<-ctx.Done()
		return nil, &net.OpError{Op: "dial", Err: ctx.Err()}
	case bNetTimeout:
		return nil, &net.OpError{Op: "dial", Err: netTimeout{}}
	case bUnreachable:
		return nil, &net.OpError{Op: "dial", Err: errors.New("no route to host")}
	default:
		return nil, &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", errConnRefused)}
	}
}

func (d *fakeDialer) callCount(addr string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls[addr]
}

func tgt(s string) Target { a := netip.MustParseAddr(s); return Target{Host: s, Addr: a} }

func collect(ch <-chan Result) []Result {
	var out []Result
	for r := range ch {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b Result) int {
		if c := a.Target.Addr.Compare(b.Target.Addr); c != 0 {
			return c
		}
		return int(a.Port) - int(b.Port)
	})
	return out
}

func summarize(rs []Result) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = fmt.Sprintf("%s:%d=%s", r.Target.Addr, r.Port, r.State)
	}
	return strings.Join(parts, " ")
}

func fastOpts(d Dialer) Options {
	return Options{Dialer: d, Timeout: 20 * time.Millisecond, Rate: -1, Concurrency: 8}
}

func TestScan(t *testing.T) {
	t.Parallel()
	d := newFakeDialer(map[string]behavior{
		"10.0.0.1:22":  bOpen,
		"10.0.0.1:80":  bTimeout,
		"10.0.0.2:22":  bNetTimeout,
		"10.0.0.2:443": bUnreachable,
		"10.0.0.2:80":  bOpen,
	})
	rs := collect(Scan(t.Context(), []Target{tgt("10.0.0.1"), tgt("10.0.0.2")}, portspec.MustParse("22,80,443"), fastOpts(d)))
	want := "10.0.0.1:22=open 10.0.0.1:80=filtered 10.0.0.1:443=closed 10.0.0.2:22=filtered 10.0.0.2:80=open 10.0.0.2:443=error"
	if got := summarize(rs); got != want {
		t.Errorf("results:\n got %s\nwant %s", got, want)
	}
	for _, r := range rs {
		if (r.State == Error) != (r.Err != nil) {
			t.Errorf("%s:%d state %s has Err %v", r.Target.Addr, r.Port, r.State, r.Err)
		}
		if r.Port == 22 && r.Service != "ssh" {
			t.Errorf("service for 22 = %q", r.Service)
		}
		if r.State == Filtered && r.Port == 80 && r.Latency < 15*time.Millisecond {
			t.Errorf("filtered latency %v shorter than timeout", r.Latency)
		}
	}
}

func TestScanRetries(t *testing.T) {
	t.Parallel()
	d := newFakeDialer(map[string]behavior{"10.0.0.1:1": bNetTimeout, "10.0.0.1:2": bOpen})
	opts := fastOpts(d)
	opts.Retries = 2
	rs := collect(Scan(t.Context(), []Target{tgt("10.0.0.1")}, portspec.MustParse("1-3"), opts))
	if len(rs) != 3 {
		t.Fatalf("got %d results", len(rs))
	}
	for addr, want := range map[string]int{"10.0.0.1:1": 3, "10.0.0.1:2": 1, "10.0.0.1:3": 1} {
		if got := d.callCount(addr); got != want {
			t.Errorf("%s dialed %d times, want %d", addr, got, want)
		}
	}
}

func TestScanCancelDuringRateLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	d := newFakeDialer(nil)
	opts := Options{Dialer: d, Concurrency: 1, Rate: 1} // burst 1, then one per second
	ch := Scan(ctx, []Target{tgt("10.0.0.1")}, portspec.MustParse("1-100"), opts)
	first := <-ch
	cancel()
	rest := collect(ch)
	if first.State != Closed || len(rest) != 0 {
		t.Errorf("first = %+v, then %d more results after cancel", first, len(rest))
	}
}

func TestScanCancelDuringDial(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	d := newFakeDialer(nil)
	d.onDial = cancel // the dial fails after the scan was cancelled
	rs := collect(Scan(ctx, []Target{tgt("10.0.0.1")}, portspec.MustParse("1-50"), fastOpts(d)))
	if len(rs) != 0 {
		t.Errorf("got %d results from a cancelled scan", len(rs))
	}
}

func TestScanCancelWhileBlockedOnSend(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	d := newFakeDialer(map[string]behavior{})
	opts := Options{Dialer: d, Concurrency: 1, Rate: -1}
	ch := Scan(ctx, []Target{tgt("10.0.0.1")}, portspec.MustParse("1-1000"), opts)
	// Don't read: the worker fills the buffer (1) and blocks on the next send.
	deadline := time.Now().Add(2 * time.Second)
	for d.callCount("10.0.0.1:2") == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
	cancel()
	n := len(collect(ch))
	if n == 0 || n > 3 {
		t.Errorf("got %d results, want the few produced before cancel", n)
	}
}

func TestScanEmpty(t *testing.T) {
	t.Parallel()
	if rs := collect(Scan(t.Context(), nil, portspec.MustParse("80"), fastOpts(newFakeDialer(nil)))); len(rs) != 0 {
		t.Errorf("got %d results", len(rs))
	}
}

func TestOptionsDefaults(t *testing.T) {
	t.Parallel()
	o := Options{Retries: -3}.withDefaults()
	if o.Concurrency != DefaultConcurrency || o.Timeout != DefaultTimeout || o.Rate != DefaultRate || o.Retries != 0 {
		t.Errorf("defaults = %+v", o)
	}
	if _, ok := o.Dialer.(*net.Dialer); !ok {
		t.Errorf("default dialer = %T", o.Dialer)
	}
	custom := Options{Concurrency: 3, Timeout: time.Minute, Rate: 7, Retries: 2, Dialer: newFakeDialer(nil)}.withDefaults()
	if custom.Concurrency != 3 || custom.Timeout != time.Minute || custom.Rate != 7 || custom.Retries != 2 {
		t.Errorf("custom options changed: %+v", custom)
	}
	if got := (Options{Rate: -1}).limiter().Limit(); got != rate.Inf {
		t.Errorf("negative rate limit = %v, want Inf", got)
	}
	if l := (Options{Rate: 50, Concurrency: 4}).limiter(); l.Limit() != 50 || l.Burst() != 4 {
		t.Errorf("limiter = %v/%d", l.Limit(), l.Burst())
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()
	other := errors.New("boom")
	tests := []struct {
		err     error
		want    State
		wantErr bool
	}{
		{&net.OpError{Err: os.NewSyscallError("connect", errConnRefused)}, Closed, false},
		{context.DeadlineExceeded, Filtered, false},
		{&net.OpError{Err: netTimeout{}}, Filtered, false},
		{other, Error, true},
	}
	for _, tt := range tests {
		st, err := classify(tt.err)
		if st != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("classify(%v) = %s, %v", tt.err, st, err)
		}
	}
}

func TestSummary(t *testing.T) {
	t.Parallel()
	var s Summary
	for _, st := range []State{Open, Open, Closed, Filtered, Error, "weird"} {
		s.Add(Result{State: st})
	}
	if s.Open != 2 || s.Closed != 1 || s.Filtered != 1 || s.Errored != 1 || s.Probes() != 5 {
		t.Errorf("summary = %+v, probes %d", s, s.Probes())
	}
}

func TestService(t *testing.T) {
	t.Parallel()
	if Service(5432) != "postgresql" || Service(443) != "https" || Service(4) != "" {
		t.Error("service table lookup wrong")
	}
}

func BenchmarkScan(b *testing.B) {
	d := newFakeDialer(map[string]behavior{"10.0.0.1:80": bOpen})
	targets := []Target{tgt("10.0.0.1"), tgt("10.0.0.2"), tgt("10.0.0.3"), tgt("10.0.0.4")}
	ports := portspec.MustParse("1-1024")
	opts := Options{Dialer: d, Rate: -1}
	b.ReportAllocs()
	for b.Loop() {
		for range Scan(context.Background(), targets, ports, opts) {
		}
	}
	b.ReportMetric(float64(len(targets)*ports.Len()), "probes/op")
}
