package cli

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// waitDialer reports an address as open after readyAfter[addr] attempts
// (0 = immediately, -1 = never).
type waitDialer struct {
	mu         sync.Mutex
	readyAfter map[string]int
	attempts   map[string]int
	onDial     func()
}

func (d *waitDialer) DialContext(_ context.Context, network, addr string) (net.Conn, error) {
	d.mu.Lock()
	d.attempts[addr]++
	n, after, onDial := d.attempts[addr], d.readyAfter[addr], d.onDial
	d.mu.Unlock()
	if onDial != nil {
		onDial()
	}
	if network != "tcp" || after < 0 || n <= after {
		return nil, errors.New("connection refused")
	}
	c1, c2 := net.Pipe()
	_ = c2.Close()
	return c1, nil
}

// freeAfterFinder reports the port free from the n-th check on.
type freeAfterFinder struct {
	fakeFinder
	mu    sync.Mutex
	after int
	calls int
	err   error
}

func (f *freeAfterFinder) CheckAll(_ context.Context, spec portspec.Spec, proto inspect.Proto) ([]freeport.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	p, _ := spec.At(0)
	st := freeport.Status{Port: p, Proto: proto, Availability: freeport.InUse}
	if f.after >= 0 && f.calls > f.after {
		st.Availability = freeport.Free
	}
	return []freeport.Status{st}, nil
}

func waitHarness(readyAfter map[string]int) (*harness, *waitDialer) {
	h := newHarness()
	d := &waitDialer{readyAfter: readyAfter, attempts: map[string]int{}}
	h.app.Dialer = d
	return h, d
}

func TestWait(t *testing.T) {
	t.Parallel()
	fast := []string{"--interval", "10ms"}
	tests := []struct {
		name       string
		args       []string
		readyAfter map[string]int
		wantCode   int
		wantOut    []string
		wantErr    []string
		check      func(t *testing.T, d *waitDialer)
	}{
		{
			name: "already open", args: []string{"wait", "5432"}, readyAfter: map[string]int{"localhost:5432": 0},
			wantOut: []string{"5432", "open", "ready", "1\n"}, wantErr: []string{"Waiting for localhost:5432 to accept connections (timeout 30s)…"},
		},
		{
			name: "opens after retries", args: append([]string{"wait", "5432"}, fast...), readyAfter: map[string]int{"localhost:5432": 3},
			wantOut: []string{"ready", "4\n"},
		},
		{
			name: "times out", args: append([]string{"wait", "5432", "-t", "50ms"}, fast...), readyAfter: map[string]int{"localhost:5432": -1},
			wantCode: ExitNegative, wantOut: []string{"timeout"},
		},
		{
			name: "host:port and --host", args: append([]string{"wait", "db:6379", "8080", "--host", "web", "-o", "csv"}, fast...),
			readyAfter: map[string]int{"db:6379": 0, "web:8080": 1},
			wantOut:    []string{"db:6379,open,ready", "8080,open,ready"}, wantErr: []string{"Waiting for 2 targets"},
			check: func(t *testing.T, d *waitDialer) {
				if d.attempts["web:8080"] != 2 || d.attempts["db:6379"] != 1 {
					t.Errorf("attempts = %v", d.attempts)
				}
			},
		},
		{
			name: "ipv6 target", args: []string{"wait", "[::1]:80"}, readyAfter: map[string]int{"[::1]:80": 0},
			wantOut: []string{"[::1]:80"},
		},
		{
			name: "one of two times out", args: append([]string{"wait", "1", "2", "-t", "50ms", "--json"}, fast...),
			readyAfter: map[string]int{"localhost:1": 0, "localhost:2": -1}, wantCode: ExitNegative,
			wantOut: []string{`"kind": "wait"`, `"ready": true`, `"ready": false`},
		},
		{
			name: "no timeout", args: []string{"wait", "9", "-t", "0"}, readyAfter: map[string]int{"localhost:9": 0},
			wantErr: []string{"(no timeout)"},
		},
		{name: "verbose", args: []string{"wait", "9", "-v"}, readyAfter: map[string]int{"localhost:9": 0}, wantErr: []string{"wait finished", "target=9"}},
		{name: "no args", args: []string{"wait"}, wantCode: ExitUsage},
		{name: "negative timeout", args: []string{"wait", "1", "-t", "-1s"}, wantCode: ExitUsage, wantErr: []string{"--timeout must not be negative"}},
		{name: "interval too small", args: []string{"wait", "1", "--interval", "1ms"}, wantCode: ExitUsage, wantErr: []string{"--interval must be at least 10ms"}},
		{name: "bad port", args: []string{"wait", "http"}, wantCode: ExitUsage, wantErr: []string{`invalid port in "http"`}},
		{name: "port out of range", args: []string{"wait", "db:70000"}, wantCode: ExitUsage, wantErr: []string{`invalid port in "db:70000"`}},
		{name: "empty host", args: []string{"wait", ":5432"}, wantCode: ExitUsage, wantErr: []string{`invalid target ":5432"`}},
		{name: "malformed target", args: []string{"wait", "a:b:c"}, wantCode: ExitUsage, wantErr: []string{`invalid target "a:b:c"`}},
		{name: "free with host:port", args: []string{"wait", "db:5432", "--free"}, wantCode: ExitUsage, wantErr: []string{"--free only works with local ports"}},
		{name: "free with --host", args: []string{"wait", "5432", "--free", "--host", "db"}, wantCode: ExitUsage, wantErr: []string{"drop --host"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, d := waitHarness(tt.readyAfter)
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
			for _, s := range tt.wantErr {
				if !strings.Contains(errOut, s) {
					t.Errorf("stderr missing %q:\n%s", s, errOut)
				}
			}
			if tt.check != nil {
				tt.check(t, d)
			}
		})
	}
}

func TestWaitFree(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		after    int
		err      error
		wantCode int
		want     string
	}{
		{"free after a while", 2, nil, ExitOK, "free       ready"},
		{"never free", -1, nil, ExitNegative, "timeout"},
		{"check errors keep waiting", 0, errors.New("lsof failed"), ExitNegative, "timeout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, d := waitHarness(nil)
			f := &freeAfterFinder{after: tc.after, err: tc.err}
			h.app.Ports = f
			code := h.run(t, "wait", "3000", "--free", "-t", "100ms", "--interval", "10ms")
			if code != tc.wantCode || !strings.Contains(h.stdout.String(), tc.want) {
				t.Errorf("exit = %d, stdout:\n%s", code, h.stdout)
			}
			if !strings.Contains(h.stderr.String(), "to be free") {
				t.Errorf("stderr = %q", h.stderr)
			}
			if len(d.attempts) != 0 {
				t.Error("--free must not dial")
			}
		})
	}
}

func TestWaitInterrupted(t *testing.T) {
	t.Parallel()
	h, d := waitHarness(map[string]int{"localhost:1": -1})
	ctx, cancel := context.WithCancel(t.Context())
	var once sync.Once
	d.onDial = func() { once.Do(cancel) }
	code := Execute(ctx, h.app, []string{"wait", "1", "--interval", "10ms"})
	if code != ExitInterrupted {
		t.Errorf("exit = %d, want %d", code, ExitInterrupted)
	}
	if !strings.Contains(h.stdout.String(), "timeout") || strings.Contains(h.stderr.String(), "portcheck:") {
		t.Errorf("stdout:\n%s\nstderr:\n%s", h.stdout, h.stderr)
	}
}

func TestWaitPrintError(t *testing.T) {
	t.Parallel()
	h, _ := waitHarness(map[string]int{"localhost:1": 0})
	h.app.Stdout = failWriter{}
	if code := h.run(t, "wait", "1"); code != ExitRuntime {
		t.Errorf("exit = %d", code)
	}
}

func TestDescribeTargets(t *testing.T) {
	t.Parallel()
	one := []waitTarget{{host: "::1", port: 80}}
	if got := describeTargets(one); got != "[::1]:80" {
		t.Errorf("got %q", got)
	}
	if got := describeTargets(append(one, one...)); got != "2 targets" {
		t.Errorf("got %q", got)
	}
}
