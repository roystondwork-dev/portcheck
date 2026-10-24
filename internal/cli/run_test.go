package cli

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

type fakeRunner struct {
	code     int
	err      error
	got      RunSpec
	called   bool
	duration time.Duration // how long the "command" runs after starting
}

func (r *fakeRunner) Run(spec RunSpec, started func()) (int, error) {
	r.called, r.got = true, spec
	if r.err != nil && r.code >= exitCannotExecute {
		return r.code, r.err // failed to start
	}
	if started != nil {
		started()
	}
	time.Sleep(r.duration)
	return r.code, r.err
}

// runFinder reports ports in busy as in use (holders from holders), others free.
type runFinder struct {
	fakeFinder
	busy    map[uint16]freeport.Availability
	holders []inspect.Socket
	checkN  int
}

func (f *runFinder) CheckAll(_ context.Context, spec portspec.Spec, proto inspect.Proto) ([]freeport.Status, error) {
	f.checkN++
	if f.checkErr != nil {
		return nil, f.checkErr
	}
	p, _ := spec.At(0)
	st := freeport.Status{Port: p, Proto: proto}
	if a, ok := f.busy[p]; ok {
		st.Availability, st.Holders = a, f.holders
	}
	return []freeport.Status{st}, nil
}

func (f *runFinder) Find(_ context.Context, spec portspec.Spec, _ inspect.Proto, _ int) ([]uint16, error) {
	f.gotSpec = spec
	if f.findErr != nil {
		return nil, f.findErr
	}
	for p := range spec.All() {
		if _, ok := f.busy[p]; !ok {
			return []uint16{p}, nil
		}
	}
	return nil, freeport.ErrNoFreePort
}

func runHarness(busy map[uint16]freeport.Availability) (*harness, *fakeRunner, *runFinder) {
	h := newHarness()
	r := &fakeRunner{}
	// Copy: tests mutate busy (e.g. a port frees after --kill) and must not
	// affect each other.
	f := &runFinder{busy: maps.Clone(busy), holders: []inspect.Socket{{RawSocket: nodeSock, Process: &inspect.ProcessInfo{PID: 100, Name: "node"}}}}
	h.app.Runner, h.app.Ports = r, f
	h.app.Environ = []string{"HOME=/home/a", "PORT=1"}
	return h, r, f
}

func TestRun(t *testing.T) {
	t.Parallel()
	inUse := map[uint16]freeport.Availability{3000: freeport.InUse}
	tests := []struct {
		name     string
		args     []string
		busy     map[uint16]freeport.Availability
		setup    func(h *harness, r *fakeRunner, f *runFinder)
		wantCode int
		wantErr  []string
		notErr   []string
		check    func(t *testing.T, h *harness, r *fakeRunner, f *runFinder)
	}{
		{
			name: "free port", args: []string{"run", "--port", "3000", "--", "npm", "start", "--port={port}"},
			wantErr: []string{"portcheck: starting npm start --port=3000 with PORT=3000"},
			check: func(t *testing.T, _ *harness, r *fakeRunner, _ *runFinder) {
				if r.got.Name != "npm" || strings.Join(r.got.Args, " ") != "start --port=3000" {
					t.Errorf("spec = %+v", r.got)
				}
				if strings.Join(r.got.Env, " ") != "HOME=/home/a PORT=3000" {
					t.Errorf("env = %v (PORT must replace the existing value)", r.got.Env)
				}
			},
		},
		{
			name: "auto port from range", args: []string{"run", "--range", "4000-4010", "srv", "{port}"},
			busy: map[uint16]freeport.Availability{4000: freeport.InUse},
			check: func(t *testing.T, _ *harness, r *fakeRunner, f *runFinder) {
				if f.gotSpec.String() != "4000-4010" || r.got.Args[0] != "4001" {
					t.Errorf("spec %s args %v", f.gotSpec, r.got.Args)
				}
			},
		},
		{
			name: "default range", args: []string{"run", "srv"},
			check: func(t *testing.T, _ *harness, _ *fakeRunner, f *runFinder) {
				if f.gotSpec.String() != defaultRunRange {
					t.Errorf("range = %s", f.gotSpec)
				}
			},
		},
		{
			name: "custom env and child flags not parsed", args: []string{"run", "-p", "3001", "--env", "HTTP_PORT", "srv", "--port", "x", "-v"},
			check: func(t *testing.T, _ *harness, r *fakeRunner, _ *runFinder) {
				if !strings.Contains(strings.Join(r.got.Env, " "), "HTTP_PORT=3001") || strings.Join(r.got.Args, " ") != "--port x -v" {
					t.Errorf("spec = %+v", r.got)
				}
			},
		},
		{
			name: "quoted display", args: []string{"run", "-p", "3001", "sh", "-c", "echo $PORT", ""},
			wantErr: []string{`starting sh -c "echo $PORT" "" with PORT=3001`},
		},
		{
			name: "quiet", args: []string{"run", "-q", "-p", "3001", "srv"},
			notErr: []string{"portcheck:"},
		},
		{
			name: "exit code passthrough", args: []string{"run", "-p", "3001", "srv"},
			setup:    func(_ *harness, r *fakeRunner, _ *runFinder) { r.code = 42 },
			wantCode: 42, notErr: []string{"portcheck: exit"},
		},
		{
			name: "command not found", args: []string{"run", "-p", "3001", "nope"},
			setup: func(_ *harness, r *fakeRunner, _ *runFinder) {
				r.code, r.err = exitNotFound, errors.New("executable file not found")
			},
			wantCode: exitNotFound, wantErr: []string{"portcheck: cannot run nope: executable file not found"},
		},
		{
			name: "runner failure after start", args: []string{"run", "-p", "3001", "srv"},
			setup:    func(_ *harness, r *fakeRunner, _ *runFinder) { r.code, r.err = ExitRuntime, errors.New("wait failed") },
			wantCode: ExitRuntime, wantErr: []string{"run srv: wait failed"},
		},
		{
			name: "busy without flags", args: []string{"run", "-p", "3000", "srv"}, busy: inUse,
			wantCode: ExitNegative, wantErr: []string{"port 3000 is in use by node (pid 100). Use --next to pick another port, or --kill to stop the holder."},
			check: func(t *testing.T, _ *harness, r *fakeRunner, _ *runFinder) {
				if r.called {
					t.Error("command started although the port is busy")
				}
			},
		},
		{
			name: "busy --next", args: []string{"run", "-p", "3000", "--next", "srv"}, busy: map[uint16]freeport.Availability{3000: freeport.InUse, 3001: freeport.InUse},
			wantErr: []string{"port 3000 is in use by node (pid 100); using 3002"},
		},
		{
			name: "busy --next at top", args: []string{"run", "-p", "65535", "--next", "srv"}, busy: map[uint16]freeport.Availability{65535: freeport.InUse},
			wantCode: ExitNegative, wantErr: []string{"no higher port"},
		},
		{
			name: "no free port in range", args: []string{"run", "--range", "5000-5001", "srv"}, busy: map[uint16]freeport.Availability{5000: freeport.InUse, 5001: freeport.InUse},
			wantCode: ExitNegative, wantErr: []string{"no free port in 5000-5001"},
		},
		{
			name: "find error", args: []string{"run", "srv"},
			setup:    func(_ *harness, _ *fakeRunner, f *runFinder) { f.findErr = errors.New("lsof failed") },
			wantCode: ExitRuntime,
		},
		{
			name: "check error", args: []string{"run", "-p", "3000", "srv"},
			setup:    func(_ *harness, _ *fakeRunner, f *runFinder) { f.checkErr = errors.New("lsof failed") },
			wantCode: ExitRuntime,
		},
		{
			name: "busy --kill", args: []string{"run", "-p", "3000", "--kill", "--yes", "--grace", "2s", "srv"}, busy: inUse,
			setup: func(h *harness, _ *fakeRunner, f *runFinder) {
				h.killer.onKill = func() { delete(f.busy, 3000) } // the port frees once the holder is gone
			},
			wantErr: []string{"terminated", "starting srv with PORT=3000"},
			check: func(t *testing.T, h *harness, r *fakeRunner, _ *runFinder) {
				if h.killer.gotOpts.Grace != 2*time.Second || len(h.killer.killed) != 1 || !r.called {
					t.Errorf("killed %v opts %+v started=%v", h.killer.killed, h.killer.gotOpts, r.called)
				}
				if h.stdout.Len() != 0 {
					t.Errorf("portcheck output leaked to stdout:\n%s", h.stdout)
				}
			},
		},
		{
			name: "--kill refused", args: []string{"run", "-p", "3000", "--kill", "--yes", "srv"}, busy: inUse,
			setup: func(h *harness, _ *fakeRunner, _ *runFinder) {
				h.killer.checkErr = map[int32]error{100: killer.ErrProtected}
			},
			wantCode: ExitNegative,
		},
		{
			name: "--kill denied port", args: []string{"run", "-p", "80", "--kill", "srv"}, busy: map[uint16]freeport.Availability{80: freeport.Denied},
			wantCode: ExitPermission, wantErr: []string{"port 80 can't be bound by your user"},
		},
		{name: "no command", args: []string{"run"}, wantCode: ExitUsage},
		{name: "port too big", args: []string{"run", "-p", "70000", "x"}, wantCode: ExitUsage, wantErr: []string{"--port must be between"}},
		{name: "negative port", args: []string{"run", "-p", "-1", "x"}, wantCode: ExitUsage},
		{name: "next and kill", args: []string{"run", "-p", "1", "--next", "--kill", "x"}, wantCode: ExitUsage, wantErr: []string{"can't be combined"}},
		{name: "next without port", args: []string{"run", "--next", "x"}, wantCode: ExitUsage, wantErr: []string{"need --port"}},
		{name: "bad env", args: []string{"run", "--env", "1BAD", "x"}, wantCode: ExitUsage, wantErr: []string{"not a valid environment variable name"}},
		{name: "bad grace", args: []string{"run", "--grace", "0s", "x"}, wantCode: ExitUsage},
		{name: "bad wait timeout", args: []string{"run", "--wait-timeout", "0s", "x"}, wantCode: ExitUsage},
		{name: "bad range", args: []string{"run", "--range", "9-1", "x"}, wantCode: ExitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, r, f := runHarness(tt.busy)
			if f.busy == nil {
				f.busy = map[uint16]freeport.Availability{}
			}
			if tt.setup != nil {
				tt.setup(h, r, f)
			}
			code := h.run(t, tt.args...)
			errOut := h.stderr.String()
			if code != tt.wantCode {
				t.Errorf("exit = %d, want %d\nstderr:\n%s", code, tt.wantCode, errOut)
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
				tt.check(t, h, r, f)
			}
		})
	}
}

func TestRunWait(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		ready     int // waitDialer readyAfter (-1 never)
		runFor    time.Duration
		timeout   string
		want, not string
	}{
		{"ready", 0, 200 * time.Millisecond, "5s", "portcheck: ready on localhost:3001", "nothing accepted"},
		{"timed out", -1, 300 * time.Millisecond, "20ms", "nothing accepted connections on port 3001 within 20ms", "ready on"},
		{"command exits first", -1, 0, "1h", "", "nothing accepted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, r, _ := runHarness(nil)
			r.duration = tc.runFor
			h.app.Dialer = &waitDialer{readyAfter: map[string]int{"localhost:3001": tc.ready}, attempts: map[string]int{}}
			if code := h.run(t, "run", "-p", "3001", "--wait", "--wait-timeout", tc.timeout, "srv"); code != 0 {
				t.Fatalf("exit = %d", code)
			}
			errOut := h.stderr.String()
			if tc.want != "" && !strings.Contains(errOut, tc.want) {
				t.Errorf("stderr missing %q:\n%s", tc.want, errOut)
			}
			if strings.Contains(errOut, tc.not) {
				t.Errorf("stderr contains %q:\n%s", tc.not, errOut)
			}
		})
	}
}

func TestRunKillPortStaysBusy(t *testing.T) { //nolint:paralleltest // shortens a package-level timeout
	old := runFreeTimeout
	runFreeTimeout = 50 * time.Millisecond
	t.Cleanup(func() { runFreeTimeout = old })
	h, r, _ := runHarness(map[uint16]freeport.Availability{3000: freeport.InUse})
	if code := h.run(t, "run", "-p", "3000", "--kill", "--yes", "srv"); code != ExitNegative {
		t.Errorf("exit = %d", code)
	}
	if !strings.Contains(h.stderr.String(), "still in use after stopping its holder") || r.called {
		t.Errorf("stderr:\n%s\nstarted=%v", h.stderr, r.called)
	}
}

func TestDescribeBusy(t *testing.T) {
	t.Parallel()
	ctr := inspect.Socket{Container: &inspect.Container{Name: "web", Runtime: "docker"}}
	proc := inspect.Socket{RawSocket: inspect.RawSocket{PID: 7}, Process: &inspect.ProcessInfo{Name: "node"}}
	pidOnly := inspect.Socket{RawSocket: inspect.RawSocket{PID: 8}}
	hidden := inspect.Socket{}
	for _, tc := range []struct {
		st   freeport.Status
		want string
	}{
		{freeport.Status{Availability: freeport.Denied}, "not usable by your user (permission denied)"},
		{freeport.Status{Availability: freeport.InUse}, "in use"},
		{freeport.Status{Availability: freeport.InUse, Holders: []inspect.Socket{hidden}}, "in use"},
		{freeport.Status{Availability: freeport.InUse, Holders: []inspect.Socket{ctr, proc, pidOnly}}, "in use by docker:web, node (pid 7), pid 8"},
		// IPv4 and IPv6 sockets of one process: named once.
		{freeport.Status{Availability: freeport.InUse, Holders: []inspect.Socket{proc, proc}}, "in use by node (pid 7)"},
	} {
		if got := describeBusy(tc.st); got != tc.want {
			t.Errorf("describeBusy = %q, want %q", got, tc.want)
		}
	}
}

func TestSubstituteAndEnv(t *testing.T) {
	t.Parallel()
	if got := substitutePort([]string{"{port}", "a{port}b{port}", "x"}, 80); strings.Join(got, ",") != "80,a80b80,x" {
		t.Errorf("substitutePort = %v", got)
	}
	if got := withEnv(nil, "P", 1); strings.Join(got, ",") != "P=1" {
		t.Errorf("withEnv = %v", got)
	}
	if got := displayCommand("a", []string{"b c", `d"e`, "f"}); got != `a "b c" "d\"e" f` {
		t.Errorf("displayCommand = %s", got)
	}
}
