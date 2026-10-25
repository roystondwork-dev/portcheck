package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/internal/buildinfo"
	"github.com/roystondwork-dev/portcheck/internal/nativehost"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// ---- fakes ---------------------------------------------------------------

type listerFunc func(ctx context.Context, f inspect.Filter) (*inspect.Result, error)

func (fn listerFunc) List(ctx context.Context, f inspect.Filter) (*inspect.Result, error) {
	return fn(ctx, f)
}

type fakeFinder struct {
	statuses  []freeport.Status
	checkErr  error
	found     []uint16
	findErr   error
	random    uint16
	randomErr error

	gotSpec  portspec.Spec
	gotProto inspect.Proto
	gotN     int
}

func (f *fakeFinder) CheckAll(_ context.Context, spec portspec.Spec, proto inspect.Proto) ([]freeport.Status, error) {
	f.gotSpec, f.gotProto = spec, proto
	return f.statuses, f.checkErr
}

func (f *fakeFinder) Find(_ context.Context, spec portspec.Spec, proto inspect.Proto, n int) ([]uint16, error) {
	f.gotSpec, f.gotProto, f.gotN = spec, proto, n
	return f.found, f.findErr
}

func (f *fakeFinder) Random(_ context.Context, spec portspec.Spec, proto inspect.Proto) (uint16, error) {
	f.gotSpec, f.gotProto = spec, proto
	return f.random, f.randomErr
}

type fakeKiller struct {
	mu       sync.Mutex
	names    map[int32]string
	checkErr map[int32]error
	killErr  map[int32]error
	outcome  killer.Outcome
	killed   []int32
	gotOpts  killer.Options
	onKill   func() // called after a successful kill
}

func (k *fakeKiller) Check(_ context.Context, pid int32, _ bool) (string, error) {
	return k.names[pid], k.checkErr[pid]
}

func (k *fakeKiller) Kill(_ context.Context, pid int32, opts killer.Options) (killer.Result, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.gotOpts = opts
	res := killer.Result{PID: pid, Name: k.names[pid], Outcome: k.outcome}
	if err := k.killErr[pid]; err != nil {
		return res, err
	}
	if opts.DryRun {
		res.Outcome = killer.DryRun
	}
	k.killed = append(k.killed, pid)
	if k.onKill != nil {
		k.onKill()
	}
	return res, nil
}

// ---- harness -------------------------------------------------------------

func ap(s string) netip.AddrPort { return netip.MustParseAddrPort(s) }

var (
	nodeSock   = inspect.RawSocket{Proto: inspect.TCP, Local: ap("127.0.0.1:3000"), State: inspect.StateListen, PID: 100}
	nodeSock6  = inspect.RawSocket{Proto: inspect.TCP, Local: ap("[::1]:3000"), State: inspect.StateListen, PID: 100}
	javaSock   = inspect.RawSocket{Proto: inspect.TCP, Local: ap("0.0.0.0:8080"), State: inspect.StateListen, PID: 200}
	hiddenSock = inspect.RawSocket{Proto: inspect.TCP, Local: ap("0.0.0.0:22"), State: inspect.StateListen}
	// testNow is the harness's clock.
	testNow   = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	ghostSock = inspect.RawSocket{Proto: inspect.TCP, Local: ap("0.0.0.0:9000"), State: inspect.StateListen, PID: 300} // process gone
)

type harness struct {
	app            *App
	stdout, stderr *bytes.Buffer
	provider       *inspect.FakeProvider
	finder         *fakeFinder
	killer         *fakeKiller
}

func newHarness() *harness {
	h := &harness{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		provider: &inspect.FakeProvider{
			SocketList: []inspect.RawSocket{nodeSock, nodeSock6, javaSock, hiddenSock, ghostSock},
			Processes: map[int32]*inspect.ProcessInfo{
				100: {PID: 100, Name: "node", User: "alice", Cmdline: "node server.js", Ancestors: []inspect.ProcessRef{{PID: 90, Name: "npm"}, {PID: 80, Name: "zsh"}}},
				200: {PID: 200, Name: "java", User: "bob"},
			},
		},
		finder: &fakeFinder{},
		killer: &fakeKiller{names: map[int32]string{100: "node", 200: "java", 300: "ghost", 1: "launchd"}, outcome: killer.Terminated},
	}
	h.app = &App{
		Stdin:         strings.NewReader(""),
		Stdout:        h.stdout,
		Stderr:        h.stderr,
		Lister:        inspect.New(h.provider),
		Ports:         h.finder,
		Killer:        h.killer,
		PrivilegeHint: func() string { return "re-run with sudo" },
		Workdir:       func() (string, error) { return "/nonexistent/portcheck-test-dir", nil },
		BrowserEnv: func() (nativehost.Env, error) {
			return nativehost.Env{GOOS: "linux", Home: "/nonexistent/portcheck-test-home"}, nil
		},
		Executable:  func() (string, error) { return "/nonexistent/portcheck", nil },
		LookPath:    func(string) (string, error) { return "", errors.New("not found") },
		Build:       buildinfo.Info{Version: "v1.2.3", Commit: "abc", Date: "2026-10-20", GoVersion: "go1.27", Platform: "test/arch"},
		Now:         func() time.Time { return testNow },
		CurrentUser: func() (string, error) { return "alice", nil },
	}
	return h
}

func (h *harness) run(t *testing.T, args ...string) int {
	t.Helper()
	return Execute(t.Context(), h.app, args)
}

type runCase struct {
	name     string
	args     []string
	setup    func(h *harness)
	wantCode int
	wantOut  []string
	wantErr  []string
	notOut   []string
	notErr   []string
	check    func(t *testing.T, h *harness)
}

func runCases(t *testing.T, cases []runCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness()
			if tc.setup != nil {
				tc.setup(h)
			}
			code := h.run(t, tc.args...)
			out, errOut := h.stdout.String(), h.stderr.String()
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, out, errOut)
			}
			for _, s := range tc.wantOut {
				if !strings.Contains(out, s) {
					t.Errorf("stdout missing %q:\n%s", s, out)
				}
			}
			for _, s := range tc.wantErr {
				if !strings.Contains(errOut, s) {
					t.Errorf("stderr missing %q:\n%s", s, errOut)
				}
			}
			for _, s := range tc.notOut {
				if strings.Contains(out, s) {
					t.Errorf("stdout unexpectedly contains %q:\n%s", s, out)
				}
			}
			for _, s := range tc.notErr {
				if strings.Contains(errOut, s) {
					t.Errorf("stderr unexpectedly contains %q:\n%s", s, errOut)
				}
			}
			if tc.check != nil {
				tc.check(t, h)
			}
		})
	}
}

func failingLister(err error) SocketLister {
	return listerFunc(func(context.Context, inspect.Filter) (*inspect.Result, error) { return nil, err })
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

// ---- root & global flags ----------------------------------------------------

func TestRoot(t *testing.T) {
	t.Parallel()
	runCases(t, []runCase{
		{name: "no args prints help", wantOut: []string{"Usage:", "portcheck [port...]"}},
		{name: "port shortcut runs check", args: []string{"3000"}, setup: func(h *harness) {
			h.finder.statuses = []freeport.Status{{Port: 3000, Proto: inspect.TCP, Availability: freeport.Free}}
		}, wantOut: []string{"3000", "free"}, check: func(t *testing.T, h *harness) {
			if h.finder.gotSpec.String() != "3000" || h.finder.gotProto != inspect.TCP {
				t.Errorf("CheckAll got %s/%s", h.finder.gotSpec, h.finder.gotProto)
			}
		}},
		{name: "typo suggests command", args: []string{"lsit"}, wantCode: ExitUsage, wantErr: []string{`unknown command "lsit"`, "Did you mean this?", "list", "Run 'portcheck --help'"}},
		{name: "unknown word without suggestion", args: []string{"zzzzzz"}, wantCode: ExitUsage, wantErr: []string{`unknown command "zzzzzz"`}, notErr: []string{"Did you mean"}},
		{name: "unknown flag is usage error", args: []string{"list", "--bogus"}, wantCode: ExitUsage, wantErr: []string{"unknown flag: --bogus", "Run 'portcheck list --help'"}},
		{name: "version flag", args: []string{"--version"}, wantOut: []string{"v1.2.3"}},
		{name: "json conflicts with output", args: []string{"version", "--json", "-o", "csv"}, wantCode: ExitUsage, wantErr: []string{"--json conflicts with --output csv"}},
		{name: "json with output json ok", args: []string{"version", "--json", "-o", "json"}, wantOut: []string{`"schemaVersion": 1`}},
		{name: "bad output format", args: []string{"version", "-o", "yaml"}, wantCode: ExitUsage, wantErr: []string{`unknown output format "yaml"`}},
		{name: "bad log format", args: []string{"version", "--log-format", "xml"}, wantCode: ExitUsage, wantErr: []string{`unknown --log-format "xml"`}},
		{name: "verbose json logs", args: []string{"list", "-v", "--log-format", "json"}, wantErr: []string{`"msg":"listed sockets"`}},
		{name: "verbose text logs", args: []string{"list", "-v"}, wantErr: []string{"msg=\"listed sockets\""}},
		{name: "quiet by default", args: []string{"list"}, notErr: []string{"listed sockets"}},
	})
}

func TestDefaultFormatDependsOnTTY(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.app.StdoutTTY = true
	if code := h.run(t, "list", "--port", "8080"); code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(h.stdout.String(), "───") {
		t.Errorf("expected table borders on a TTY:\n%s", h.stdout)
	}

	h = newHarness()
	if code := h.run(t, "list", "--port", "8080"); code != 0 {
		t.Fatal(code)
	}
	if strings.Contains(h.stdout.String(), "───") {
		t.Errorf("expected plain output when not a TTY:\n%s", h.stdout)
	}
}

// ---- list ------------------------------------------------------------------

func TestList(t *testing.T) {
	t.Parallel()
	runCases(t, []runCase{
		{name: "all with hint", args: []string{"list"}, wantOut: []string{"127.0.0.1:3000", "node", "java", "0.0.0.0:22"}, wantErr: []string{"hint: re-run with sudo"}},
		{name: "alias", args: []string{"ls", "--port", "8080"}, wantOut: []string{"java"}, notOut: []string{"node"}},
		{name: "filters", args: []string{"list", "-l", "--proto", "TCP", "--port", "3000", "--pid", "100", "--name", "NO*", "--state", "listen"}, wantOut: []string{"[::1]:3000", "127.0.0.1:3000"}, notOut: []string{"java"}, notErr: []string{"hint"}},
		{name: "no match", args: []string{"list", "--port", "1"}, wantOut: []string{"No matching sockets."}},
		{name: "csv", args: []string{"list", "--port", "8080", "-o", "csv"}, wantOut: []string{"PROTO,LOCAL ADDRESS", "tcp,0.0.0.0:8080,-,LISTEN,200,java,bob"}},
		{name: "wide", args: []string{"list", "--port", "3000", "-w", "-o", "csv"}, wantOut: []string{"PARENT,COMMAND", "node,alice,npm (90),node server.js"}},
		{name: "json", args: []string{"list", "--port", "8080", "--json"}, wantOut: []string{`"kind": "list"`, `"localPort": 8080`, `"partial": false`}},
		{name: "bad proto", args: []string{"list", "--proto", "sctp"}, wantCode: ExitUsage, wantErr: []string{`unknown protocol "sctp"`}},
		{name: "bad port", args: []string{"list", "--port", "0"}, wantCode: ExitUsage, wantErr: []string{"out of range"}},
		{name: "bad name glob", args: []string{"list", "--name", "["}, wantCode: ExitUsage, wantErr: []string{"invalid filter"}},
		{name: "negative watch", args: []string{"list", "--watch", "-1s"}, wantCode: ExitUsage, wantErr: []string{"--watch must be positive"}},
		{name: "positional args rejected", args: []string{"list", "3000"}, wantCode: ExitUsage, wantErr: []string{"unknown command"}},
		{name: "lister error", args: []string{"list"}, setup: func(h *harness) { h.app.Lister = failingLister(errors.New("lsof exploded")) }, wantCode: ExitRuntime, wantErr: []string{"portcheck: lsof exploded"}},
		{name: "lister permission error", args: []string{"list"}, setup: func(h *harness) { h.app.Lister = failingLister(inspect.ErrAccessDenied) }, wantCode: ExitPermission},
		{name: "print error", args: []string{"list"}, setup: func(h *harness) { h.app.Stdout = failWriter{} }, wantCode: ExitRuntime, wantErr: []string{"broken pipe"}},
		{name: "empty hint prints nothing", args: []string{"list"}, setup: func(h *harness) { h.app.PrivilegeHint = func() string { return "" } }, notErr: []string{"hint"}},
	})
}

func TestListWatch(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.app.StdoutTTY = true
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	h.app.Lister = listerFunc(func(context.Context, inspect.Filter) (*inspect.Result, error) {
		calls++
		if calls == 3 {
			cancel()
		}
		return &inspect.Result{Partial: true}, nil
	})
	if code := Execute(ctx, h.app, []string{"list", "--watch", "5ms"}); code != ExitOK {
		t.Fatalf("exit code = %d", code)
	}
	if got := strings.Count(h.stdout.String(), "\x1b[H\x1b[2J"); got != 3 {
		t.Errorf("screen cleared %d times, want 3", got)
	}
	if got := strings.Count(h.stderr.String(), "hint:"); got != 1 {
		t.Errorf("hint printed %d times, want once", got)
	}
}

func TestListWatchErrorStops(t *testing.T) {
	t.Parallel()
	h := newHarness()
	calls := 0
	h.app.Lister = listerFunc(func(context.Context, inspect.Filter) (*inspect.Result, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("second call fails")
		}
		return &inspect.Result{}, nil
	})
	if code := h.run(t, "list", "--watch", "1ms"); code != ExitRuntime {
		t.Fatalf("exit code = %d", code)
	}
	if strings.Contains(h.stdout.String(), "\x1b[H") {
		t.Error("screen must not be cleared when stdout is not a TTY")
	}
}

// ---- check -----------------------------------------------------------------

func TestCheck(t *testing.T) {
	t.Parallel()
	busy := []freeport.Status{
		{Port: 3000, Proto: inspect.TCP, Availability: freeport.InUse, Holders: []inspect.Socket{{RawSocket: nodeSock, Process: &inspect.ProcessInfo{PID: 100, Name: "node"}}}},
		{Port: 3001, Proto: inspect.TCP, Availability: freeport.Free},
	}
	runCases(t, []runCase{
		{name: "all free", args: []string{"check", "3000", "3001"}, setup: func(h *harness) {
			h.finder.statuses = []freeport.Status{{Port: 3000, Availability: freeport.Free}, {Port: 3001, Availability: freeport.Free}}
		}, check: func(t *testing.T, h *harness) {
			if h.finder.gotSpec.String() != "3000-3001" {
				t.Errorf("spec = %s", h.finder.gotSpec)
			}
		}},
		{name: "busy exits 1 silently", args: []string{"check", "3000-3001"}, setup: func(h *harness) { h.finder.statuses = busy }, wantCode: ExitNegative, wantOut: []string{"in-use", "node", "free"}, notErr: []string{"portcheck:", "hint"}},
		{name: "hidden owner hints", args: []string{"check", "22"}, setup: func(h *harness) {
			h.finder.statuses = []freeport.Status{{Port: 22, Availability: freeport.InUse}}
		}, wantCode: ExitNegative, wantErr: []string{"hint: re-run with sudo"}},
		{name: "denied is not free", args: []string{"check", "80"}, setup: func(h *harness) {
			h.finder.statuses = []freeport.Status{{Port: 80, Availability: freeport.Denied}}
		}, wantCode: ExitNegative, wantOut: []string{"denied"}},
		{name: "udp", args: []string{"check", "53", "--proto", "udp"}, setup: func(h *harness) {
			h.finder.statuses = []freeport.Status{{Port: 53, Availability: freeport.Free}}
		}, check: func(t *testing.T, h *harness) {
			if h.finder.gotProto != inspect.UDP {
				t.Errorf("proto = %s", h.finder.gotProto)
			}
		}},
		{name: "bad proto", args: []string{"check", "53", "--proto", "icmp"}, wantCode: ExitUsage},
		{name: "no args", args: []string{"check"}, wantCode: ExitUsage, wantErr: []string{"requires at least 1 arg"}},
		{name: "bad spec", args: []string{"check", "http"}, wantCode: ExitUsage, wantErr: []string{"unknown preset"}},
		{name: "too many ports", args: []string{"check", "all"}, wantCode: ExitUsage, wantErr: []string{"too many ports (65535)"}},
		{name: "finder error", args: []string{"check", "1"}, setup: func(h *harness) { h.finder.checkErr = errors.New("boom") }, wantCode: ExitRuntime, wantErr: []string{"boom"}},
		{name: "print error", args: []string{"check", "1"}, setup: func(h *harness) { h.app.Stdout = failWriter{} }, wantCode: ExitRuntime},
	})
}

// ---- free ------------------------------------------------------------------

func TestFree(t *testing.T) {
	t.Parallel()
	runCases(t, []runCase{
		{name: "default range", args: []string{"free"}, setup: func(h *harness) { h.finder.found = []uint16{1024} }, wantOut: []string{"1024"}, check: func(t *testing.T, h *harness) {
			if h.finder.gotSpec.String() != defaultFreeRange || h.finder.gotN != 1 || h.finder.gotProto != inspect.TCP {
				t.Errorf("Find(%s, %s, %d)", h.finder.gotSpec, h.finder.gotProto, h.finder.gotN)
			}
		}},
		{name: "range and count", args: []string{"free", "3000-3999", "-n", "2", "--proto", "udp"}, setup: func(h *harness) { h.finder.found = []uint16{3000, 3001} }, wantOut: []string{"3000\n3001"}, check: func(t *testing.T, h *harness) {
			if h.finder.gotSpec.String() != "3000-3999" || h.finder.gotN != 2 || h.finder.gotProto != inspect.UDP {
				t.Errorf("Find(%s, %s, %d)", h.finder.gotSpec, h.finder.gotProto, h.finder.gotN)
			}
		}},
		{name: "fewer than requested", args: []string{"free", "-n", "3"}, setup: func(h *harness) {
			h.finder.found = []uint16{5000}
			h.finder.findErr = fmt.Errorf("found 1 of 3: %w", freeport.ErrNoFreePort)
		}, wantCode: ExitNegative, wantOut: []string{"5000"}, notErr: []string{"portcheck:"}},
		{name: "none found", args: []string{"free"}, setup: func(h *harness) { h.finder.findErr = freeport.ErrNoFreePort }, wantCode: ExitNegative, wantOut: []string{"No free port found."}},
		{name: "find error", args: []string{"free"}, setup: func(h *harness) { h.finder.findErr = errors.New("kaput") }, wantCode: ExitRuntime, wantErr: []string{"kaput"}},
		{name: "random", args: []string{"free", "--random", "20000-30000", "--json"}, setup: func(h *harness) { h.finder.random = 24680 }, wantOut: []string{"24680"}},
		{name: "random none", args: []string{"free", "--random"}, setup: func(h *harness) { h.finder.randomErr = freeport.ErrNoFreePort }, wantCode: ExitNegative},
		{name: "random error", args: []string{"free", "--random"}, setup: func(h *harness) { h.finder.randomErr = errors.New("rng") }, wantCode: ExitRuntime},
		{name: "random with count", args: []string{"free", "--random", "-n", "2"}, wantCode: ExitUsage, wantErr: []string{"--random returns a single port"}},
		{name: "count zero", args: []string{"free", "-n", "0"}, wantCode: ExitUsage, wantErr: []string{"--count must be at least 1"}},
		{name: "bad range", args: []string{"free", "9-1"}, wantCode: ExitUsage},
		{name: "bad proto", args: []string{"free", "--proto", "x"}, wantCode: ExitUsage},
		{name: "too many args", args: []string{"free", "1", "2"}, wantCode: ExitUsage},
		{name: "print error", args: []string{"free"}, setup: func(h *harness) { h.app.Stdout = failWriter{}; h.finder.found = []uint16{1} }, wantCode: ExitRuntime},
	})
}

// ---- kill ------------------------------------------------------------------

func TestKill(t *testing.T) {
	t.Parallel()
	runCases(t, []runCase{
		{name: "by port with yes", args: []string{"kill", "3000", "--yes"}, wantOut: []string{"100", "node", "terminated"}, check: func(t *testing.T, h *harness) {
			if fmt.Sprint(h.killer.killed) != "[100]" {
				t.Errorf("killed %v, want [100] once despite two sockets", h.killer.killed)
			}
			if h.killer.gotOpts != (killer.Options{Grace: killer.DefaultGrace}) {
				t.Errorf("opts = %+v", h.killer.gotOpts)
			}
		}},
		{name: "options passed through", args: []string{"kill", "--pid", "200", "-y", "--force", "--grace", "2s", "--allow-system"}, check: func(t *testing.T, h *harness) {
			want := killer.Options{Grace: 2 * time.Second, Force: true, AllowProtected: true}
			if h.killer.gotOpts != want {
				t.Errorf("opts = %+v, want %+v", h.killer.gotOpts, want)
			}
		}},
		{name: "ports and pids merged", args: []string{"kill", "3000", "8080", "--pid", "100", "--pid", "200", "-y", "-o", "csv"}, wantOut: []string{"100,node,terminated", "200,java,terminated"}, check: func(t *testing.T, h *harness) {
			if fmt.Sprint(h.killer.killed) != "[100 200]" {
				t.Errorf("killed %v", h.killer.killed)
			}
		}},
		{name: "dry run needs no confirmation", args: []string{"kill", "8080", "--dry-run"}, wantOut: []string{"dry-run"}},
		{name: "owner without process info", args: []string{"kill", "9000", "-y"}, wantOut: []string{"300", "ghost"}},
		{name: "nothing listening", args: []string{"kill", "4444"}, wantCode: ExitNegative, wantErr: []string{"Nothing is listening on port 4444."}},
		{name: "owner hidden", args: []string{"kill", "22"}, wantCode: ExitPermission, wantErr: []string{"hint: re-run with sudo", "port 22 is in use but its owner is not visible"}},
		{name: "no targets", args: []string{"kill"}, wantCode: ExitUsage, wantErr: []string{"specify at least one port or --pid"}},
		{name: "bad grace", args: []string{"kill", "1", "--grace", "0s"}, wantCode: ExitUsage},
		{name: "bad proto", args: []string{"kill", "1", "--proto", "x"}, wantCode: ExitUsage},
		{name: "bad port", args: []string{"kill", "x"}, wantCode: ExitUsage},
		{name: "lister error", args: []string{"kill", "1"}, setup: func(h *harness) { h.app.Lister = failingLister(errors.New("nope")) }, wantCode: ExitRuntime},
		{name: "non-interactive without yes", args: []string{"kill", "3000"}, wantCode: ExitUsage, wantErr: []string{"pass --yes"}},
		{name: "protected refused before prompt", args: []string{"kill", "--pid", "1"}, setup: func(h *harness) {
			h.killer.checkErr = map[int32]error{1: fmt.Errorf("%w: reserved", killer.ErrProtected)}
		}, wantCode: ExitNegative, wantOut: []string{"launchd", "failed", "refusing to kill protected process"}, check: func(t *testing.T, h *harness) {
			if len(h.killer.killed) != 0 {
				t.Error("protected process was killed")
			}
		}},
		{name: "guard error without name", args: []string{"kill", "--pid", "7", "-y"}, setup: func(h *harness) {
			h.killer.checkErr = map[int32]error{7: killer.ErrNotFound}
		}, wantCode: ExitNegative, wantOut: []string{"7", "?", "failed"}},
		{name: "kill permission", args: []string{"kill", "8080", "-y"}, setup: func(h *harness) {
			h.killer.killErr = map[int32]error{200: fmt.Errorf("kill: %w", killer.ErrPermission)}
		}, wantCode: ExitPermission, wantOut: []string{"failed", "permission denied"}, wantErr: []string{"hint:"}},
		{name: "kill runtime error", args: []string{"kill", "8080", "-y"}, setup: func(h *harness) {
			h.killer.killErr = map[int32]error{200: errors.New("weird")}
		}, wantCode: ExitRuntime, wantOut: []string{"weird"}},
		{name: "worst exit code wins", args: []string{"kill", "3000", "8080", "-y"}, setup: func(h *harness) {
			h.killer.killErr = map[int32]error{100: killer.ErrStillRunning, 200: fs.ErrPermission}
		}, wantCode: ExitPermission},
		{name: "print error", args: []string{"kill", "8080", "-y"}, setup: func(h *harness) { h.app.Stdout = failWriter{} }, wantCode: ExitRuntime},
	})
}

func TestKillConfirmation(t *testing.T) {
	t.Parallel()
	runCases(t, []runCase{
		{name: "yes", args: []string{"kill", "3000"}, setup: func(h *harness) { h.app.StdinTTY, h.app.Stdin = true, strings.NewReader("y\n") }, wantErr: []string{"About to terminate:", "PID 100  node  (port 3000), started by npm (90) ← zsh (80)\n", "Proceed? [y/N]"}, wantOut: []string{"terminated"}},
		{name: "YES word", args: []string{"kill", "3000"}, setup: func(h *harness) { h.app.StdinTTY, h.app.Stdin = true, strings.NewReader(" Yes \n") }, wantOut: []string{"terminated"}},
		{name: "no", args: []string{"kill", "3000"}, setup: func(h *harness) { h.app.StdinTTY, h.app.Stdin = true, strings.NewReader("n\n") }, wantCode: ExitNegative, wantErr: []string{"Aborted."}, check: func(t *testing.T, h *harness) {
			if len(h.killer.killed) != 0 {
				t.Error("killed after declining")
			}
		}},
		{name: "eof means no", args: []string{"kill", "3000"}, setup: func(h *harness) { h.app.StdinTTY = true }, wantCode: ExitNegative},
		{name: "answer without newline", args: []string{"kill", "3000"}, setup: func(h *harness) { h.app.StdinTTY, h.app.Stdin = true, strings.NewReader("y") }, wantOut: []string{"terminated"}},
		{name: "force wording and pid-only target", args: []string{"kill", "--pid", "200", "--force"}, setup: func(h *harness) { h.app.StdinTTY, h.app.Stdin = true, strings.NewReader("y\n") }, wantErr: []string{"About to force-kill:", "PID 200  java\n"}},
	})
}

func TestKillExitCode(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]int{
		killer.ErrPermission:    ExitPermission,
		killer.ErrProtected:     ExitNegative,
		killer.ErrNotFound:      ExitNegative,
		killer.ErrStillRunning:  ExitRuntime,
		inspect.ErrAccessDenied: ExitPermission,
	} {
		if got := killExitCode(err); got != want {
			t.Errorf("killExitCode(%v) = %d, want %d", err, got, want)
		}
	}
}

// ---- version ---------------------------------------------------------------

func TestVersion(t *testing.T) {
	t.Parallel()
	runCases(t, []runCase{
		{name: "text", args: []string{"version"}, wantOut: []string{"portcheck v1.2.3 (commit abc, built 2026-10-20, go1.27, test/arch)"}},
		{name: "json", args: []string{"version", "--json"}, wantOut: []string{`"kind": "version"`, `"version": "v1.2.3"`}},
		{name: "csv", args: []string{"version", "-o", "csv"}, wantOut: []string{"VERSION,COMMIT", "v1.2.3,abc"}},
		{name: "args rejected", args: []string{"version", "x"}, wantCode: ExitUsage},
	})
}

// ---- exit codes & plumbing -------------------------------------------------

func TestExitCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err  error
		want int
	}{
		{nil, ExitOK},
		{&ExitError{Code: 42}, 42},
		{fmt.Errorf("wrapped: %w", &ExitError{Code: ExitUsage}), ExitUsage},
		{context.Canceled, ExitInterrupted},
		{fs.ErrPermission, ExitPermission},
		{killer.ErrPermission, ExitPermission},
		{errors.New("x"), ExitRuntime},
	}
	for _, tt := range tests {
		if got := exitCode(tt.err); got != tt.want {
			t.Errorf("exitCode(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}

func TestExitError(t *testing.T) {
	t.Parallel()
	if got := (&ExitError{Code: 7}).Error(); got != "exit status 7" {
		t.Errorf("Error() = %q", got)
	}
	inner := errors.New("inner")
	e := &ExitError{Code: 1, Err: inner}
	if e.Error() != "inner" || !errors.Is(e, inner) {
		t.Error("ExitError does not wrap its cause")
	}
	if !isSilent(negative()) || !isSilent(context.Canceled) || isSilent(inner) || isSilent(e) {
		t.Error("isSilent wrong")
	}
}

func TestInterruptedIsSilent(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.app.Lister = failingLister(context.Canceled)
	if code := h.run(t, "list"); code != ExitInterrupted {
		t.Errorf("exit code = %d", code)
	}
	if h.stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing on interrupt", h.stderr)
	}
}

func TestNewApp(t *testing.T) {
	t.Parallel()
	app := NewApp()
	if app.Lister == nil || app.Ports == nil || app.Killer == nil || app.PrivilegeHint == nil || app.Scan == nil || app.Runner == nil || app.Workdir == nil || app.Dialer == nil || app.Resolver == nil || app.RunTUI == nil || app.BrowserEnv == nil || app.Executable == nil || app.LookPath == nil || app.Build.Version == "" {
		t.Errorf("NewApp left dependencies unset: %+v", app)
	}
	if app.Stdout != os.Stdout || app.Stderr != os.Stderr || app.Stdin != os.Stdin {
		t.Error("NewApp not wired to os streams")
	}
}

func TestMainEntrypoint(t *testing.T) { //nolint:paralleltest // swaps os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	code := Main([]string{"version", "--json"})
	os.Stdout = orig
	w.Close()
	out, _ := io.ReadAll(r)
	if code != ExitOK || !bytes.Contains(out, []byte(`"kind": "version"`)) {
		t.Errorf("Main = %d, output %q", code, out)
	}
}
