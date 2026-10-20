package cli

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
	"github.com/roystondwork-dev/portcheck/pkg/scan"
)

type fakeResolver map[string][]netip.Addr

func (r fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := r[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

// fakeScan answers every probe immediately: ports in open are open, the rest closed.
type fakeScan struct {
	open     map[string]bool // "addr:port"
	gotOpts  scan.Options
	gotPorts portspec.Spec
	gotHosts []string
	onResult func(n int) // called before each result is sent
}

func (f *fakeScan) run(ctx context.Context, targets []scan.Target, ports portspec.Spec, opts scan.Options) <-chan scan.Result {
	f.gotOpts, f.gotPorts = opts, ports
	for _, t := range targets {
		f.gotHosts = append(f.gotHosts, t.String())
	}
	ch := make(chan scan.Result)
	go func() {
		defer close(ch)
		n := 0
		// Emit in reverse so the command has to sort.
		for i := len(targets) - 1; i >= 0; i-- {
			t := targets[i]
			for p := range ports.All() {
				if ctx.Err() != nil {
					return
				}
				n++
				if f.onResult != nil {
					f.onResult(n)
				}
				st := scan.Closed
				if f.open[netip.AddrPortFrom(t.Addr, p).String()] {
					st = scan.Open
				}
				select {
				case ch <- scan.Result{Target: t, Port: p, State: st, Service: scan.Service(p), Latency: time.Millisecond}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch
}

func scanHarness(open ...string) (*harness, *fakeScan) {
	h := newHarness()
	fs := &fakeScan{open: map[string]bool{}}
	for _, o := range open {
		fs.open[o] = true
	}
	h.app.Scan = fs.run
	h.app.Resolver = fakeResolver{"db.internal": {netip.MustParseAddr("10.0.0.5")}, "public.example": {netip.MustParseAddr("8.8.8.8")}}
	return h, fs
}

func TestScan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		open     []string
		wantCode int
		wantOut  []string
		wantErr  []string
		notOut   []string
		notErr   []string
		check    func(t *testing.T, fs *fakeScan, out string)
	}{
		{
			name: "open port found", args: []string{"scan", "db.internal", "-p", "5432,6379"}, open: []string{"10.0.0.5:5432"},
			wantOut: []string{"db.internal (10.0.0.5)  5432  open    postgresql", "6379  closed  redis"},
			wantErr: []string{"Scanned 1 host(s) × 2 port(s)", "1 open, 1 closed, 0 filtered, 0 errors."},
			notErr:  []string{"note:"},
		},
		{
			name: "results sorted by target order then port", args: []string{"scan", "10.0.0.2", "10.0.0.1", "-p", "2,1", "-o", "csv"}, open: []string{"10.0.0.1:1"},
			check: func(t *testing.T, _ *fakeScan, out string) {
				want := "10.0.0.2,1,closed\n10.0.0.2,2,closed\n10.0.0.1,1,open\n10.0.0.1,2,closed"
				lines := strings.Split(strings.TrimSpace(out), "\n")[1:]
				got := make([]string, 0, len(lines))
				for _, l := range lines {
					f := strings.Split(l, ",")
					got = append(got, strings.Join(f[:3], ","))
				}
				if strings.Join(got, "\n") != want {
					t.Errorf("order:\n%s\nwant:\n%s", strings.Join(got, "\n"), want)
				}
			},
		},
		{
			name: "open only", args: []string{"scan", "10.0.0.1", "-p", "1-3", "--open", "-o", "plain"}, open: []string{"10.0.0.1:2"},
			wantOut: []string{"10.0.0.1  2"}, notOut: []string{"closed"}, wantErr: []string{"1 open, 2 closed"},
		},
		{name: "none open exits 1", args: []string{"scan", "10.0.0.1", "-p", "1"}, wantCode: ExitNegative, notErr: []string{"portcheck:"}},
		{name: "open only and none open", args: []string{"scan", "10.0.0.1", "-p", "1", "--open"}, wantCode: ExitNegative, wantOut: []string{"No results to show."}},
		{name: "json", args: []string{"scan", "10.0.0.1", "-p", "80", "--json"}, open: []string{"10.0.0.1:80"}, wantOut: []string{`"kind": "scan"`, `"state": "open"`, `"service": "http"`, `"open": 1`}},
		{name: "public target note", args: []string{"scan", "public.example", "-p", "53"}, wantCode: ExitNegative, wantErr: []string{authorizationNote}},
		{
			name: "options passed through", args: []string{"scan", "10.0.0.1", "-p", "common", "--concurrency", "5", "--timeout", "250ms", "--rate", "50", "--retries", "2"}, wantCode: ExitNegative,
			check: func(t *testing.T, fs *fakeScan, _ string) {
				want := scan.Options{Concurrency: 5, Timeout: 250 * time.Millisecond, Rate: 50, Retries: 2}
				if fs.gotOpts != want || fs.gotPorts.String() != portspec.MustParse("common").String() {
					t.Errorf("opts = %+v ports %s", fs.gotOpts, fs.gotPorts)
				}
			},
		},
		{
			name: "rate 0 means unlimited", args: []string{"scan", "10.0.0.1", "-p", "1", "--rate", "0"}, wantCode: ExitNegative,
			check: func(t *testing.T, fs *fakeScan, _ string) {
				if fs.gotOpts.Rate != -1 {
					t.Errorf("rate = %v, want -1", fs.gotOpts.Rate)
				}
			},
		},
		{
			name: "cidr and dedupe", args: []string{"scan", "10.0.0.0/31", "10.0.0.1", "-p", "1"}, wantCode: ExitNegative,
			check: func(t *testing.T, fs *fakeScan, _ string) {
				if strings.Join(fs.gotHosts, ",") != "10.0.0.0,10.0.0.1" {
					t.Errorf("hosts = %v", fs.gotHosts)
				}
			},
		},
		{name: "verbose logs", args: []string{"scan", "10.0.0.1", "-p", "1", "-v"}, wantCode: ExitNegative, wantErr: []string{"starting scan", "probes=1"}},
		{name: "no targets", args: []string{"scan"}, wantCode: ExitUsage},
		{name: "bad target", args: []string{"scan", "not a host"}, wantCode: ExitUsage, wantErr: []string{"invalid target"}},
		{name: "too many hosts", args: []string{"scan", "10.0.0.0/24", "--max-hosts", "10"}, wantCode: ExitUsage, wantErr: []string{"too many hosts"}},
		{name: "unresolvable", args: []string{"scan", "nope.internal"}, wantCode: ExitRuntime, wantErr: []string{"cannot resolve host"}},
		{name: "bad ports", args: []string{"scan", "10.0.0.1", "-p", "0"}, wantCode: ExitUsage},
		{name: "bad concurrency", args: []string{"scan", "x", "--concurrency", "0"}, wantCode: ExitUsage, wantErr: []string{"--concurrency"}},
		{name: "bad timeout", args: []string{"scan", "x", "--timeout", "0s"}, wantCode: ExitUsage, wantErr: []string{"--timeout"}},
		{name: "bad rate", args: []string{"scan", "x", "--rate", "-1"}, wantCode: ExitUsage, wantErr: []string{"--rate"}},
		{name: "bad retries", args: []string{"scan", "x", "--retries", "-1"}, wantCode: ExitUsage, wantErr: []string{"--retries"}},
		{name: "bad max hosts", args: []string{"scan", "x", "--max-hosts", "0"}, wantCode: ExitUsage, wantErr: []string{"--max-hosts"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, fs := scanHarness(tt.open...)
			code := h.run(t, tt.args...)
			out, errOut := h.stdout.String(), h.stderr.String()
			if code != tt.wantCode {
				t.Errorf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tt.wantCode, out, errOut)
			}
			for _, s := range tt.wantOut {
				if !strings.Contains(out, s) {
					t.Errorf("stdout missing %q:\n%s", s, out)
				}
			}
			for _, s := range tt.notOut {
				if strings.Contains(out, s) {
					t.Errorf("stdout contains %q:\n%s", s, out)
				}
			}
			for _, s := range tt.wantErr {
				if !strings.Contains(errOut, s) {
					t.Errorf("stderr missing %q:\n%s", s, errOut)
				}
			}
			for _, s := range tt.notErr {
				if strings.Contains(errOut, s) {
					t.Errorf("stderr contains %q:\n%s", s, errOut)
				}
			}
			if tt.check != nil {
				tt.check(t, fs, out)
			}
		})
	}
}

func TestScanPrintError(t *testing.T) {
	t.Parallel()
	h, _ := scanHarness()
	h.app.Stdout = failWriter{}
	if code := h.run(t, "scan", "10.0.0.1", "-p", "1"); code != ExitRuntime {
		t.Errorf("exit = %d", code)
	}
}

func TestScanInterrupted(t *testing.T) {
	t.Parallel()
	h, fs := scanHarness("10.0.0.1:1")
	ctx, cancel := context.WithCancel(t.Context())
	fs.onResult = func(n int) {
		if n == 3 {
			cancel()
		}
	}
	code := Execute(ctx, h.app, []string{"scan", "10.0.0.1", "-p", "1-100", "-o", "plain"})
	if code != ExitInterrupted {
		t.Errorf("exit = %d, want %d", code, ExitInterrupted)
	}
	if !strings.Contains(strings.Join(strings.Fields(h.stdout.String()), " "), "10.0.0.1 1 open") {
		t.Errorf("partial results not printed:\n%s", h.stdout)
	}
	if strings.Contains(h.stderr.String(), "portcheck:") {
		t.Errorf("interrupt should be silent:\n%s", h.stderr)
	}
}

func TestScanProgress(t *testing.T) {
	t.Parallel()
	h, _ := scanHarness()
	h.app.StderrTTY = true
	h.run(t, "scan", "10.0.0.1", "-p", "1-3")
	errOut := h.stderr.String()
	// First update and the final one are shown; the middle one is throttled.
	if got := strings.Count(errOut, "scanning "); got != 2 {
		t.Errorf("progress shown %d times, want 2:\n%q", got, errOut)
	}
	if !strings.Contains(errOut, "scanning 3/3 (0 open)\r\x1b[K") {
		t.Errorf("final progress not cleared:\n%q", errOut)
	}

	h, _ = scanHarness()
	h.run(t, "scan", "10.0.0.1", "-p", "1-3")
	if strings.Contains(h.stderr.String(), "\r") {
		t.Error("progress shown on a non-terminal stderr")
	}
}

func TestIsPublic(t *testing.T) {
	t.Parallel()
	for s, want := range map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "192.168.1.1": false, "172.16.0.1": false,
		"169.254.1.1": false, "fe80::1": false, "::1": false, "fd00::1": false, "0.0.0.0": false,
	} {
		if got := isPublic(netip.MustParseAddr(s)); got != want {
			t.Errorf("isPublic(%s) = %v, want %v", s, got, want)
		}
	}
}

func TestRoundDuration(t *testing.T) {
	t.Parallel()
	if got := roundDuration(1234567 * time.Nanosecond); got != time.Millisecond {
		t.Errorf("roundDuration(1.23ms) = %v", got)
	}
	if got := roundDuration(187654 * time.Nanosecond); got != 188*time.Microsecond {
		t.Errorf("roundDuration(187.654µs) = %v", got)
	}
}
