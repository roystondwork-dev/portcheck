package cli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

const doctorConfig = `version: 1
ports:
  - name: web
    port: 3000
  - name: db
    port: 5432
    expect: open
    description: docker compose up -d db
  - name: cache
    port: 6379
    expect: open
    host: 127.0.0.1
`

func projectDir(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	if config != "" {
		if err := os.WriteFile(filepath.Join(dir, ".portcheck.yaml"), []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "src")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	return sub
}

func doctorHarness(t *testing.T, config string, busy map[uint16]freeport.Availability, holders []inspect.Socket, open map[string]int) *harness {
	t.Helper()
	h, _, f := runHarness(busy)
	f.holders = holders
	dir := projectDir(t, config)
	h.app.Workdir = func() (string, error) { return dir, nil }
	h.app.Dialer = &waitDialer{readyAfter: open, attempts: map[string]int{}}
	return h
}

var (
	nodeHolder = inspect.Socket{RawSocket: nodeSock, Process: &inspect.ProcessInfo{PID: 100, Name: "node", Ancestors: []inspect.ProcessRef{{PID: 90, Name: "npm"}}}}
	ctrHolder  = inspect.Socket{RawSocket: nodeSock, Container: &inspect.Container{Name: "web", Image: "nginx", Runtime: "docker"}}
	hidHolder  = inspect.Socket{RawSocket: hiddenSock}
)

func TestDoctor(t *testing.T) {
	t.Parallel()
	allOpen := map[string]int{"localhost:5432": 0, "127.0.0.1:6379": 0}
	sudoFix := "sudo portcheck kill 3000"
	if runtime.GOOS == "windows" {
		sudoFix = "portcheck kill 3000  (from an Administrator terminal)"
	}
	tests := []struct {
		name     string
		busy     map[uint16]freeport.Availability
		holders  []inspect.Socket
		open     map[string]int
		args     []string
		wantCode int
		wantOut  []string
		wantErr  []string
	}{
		{name: "all ok", open: allOpen, wantOut: []string{"web    3000  free    ok", "db     5432  open    ok"}, wantErr: []string{"All 3 checks passed ("}},
		{
			name: "busy by process", busy: map[uint16]freeport.Availability{3000: freeport.InUse}, holders: []inspect.Socket{nodeHolder}, open: allOpen,
			wantCode: ExitNegative, wantOut: []string{"in-use", "in use by node (pid 100), started by npm (90)", "portcheck kill 3000"}, wantErr: []string{"1 of 3 checks need attention"},
		},
		{
			name: "busy by container", busy: map[uint16]freeport.Availability{3000: freeport.InUse}, holders: []inspect.Socket{ctrHolder}, open: allOpen,
			wantCode: ExitNegative, wantOut: []string{"in use by docker:web", "docker stop web"},
		},
		{
			name: "busy by hidden owner", busy: map[uint16]freeport.Availability{3000: freeport.InUse}, holders: []inspect.Socket{hidHolder}, open: allOpen,
			wantCode: ExitNegative, wantOut: []string{sudoFix},
		},
		{
			name: "denied", busy: map[uint16]freeport.Availability{3000: freeport.Denied}, open: allOpen,
			wantCode: ExitNegative, wantOut: []string{"denied", "your user can't bind this port", "use a port ≥ 1024"},
		},
		{
			name: "service down", open: map[string]int{"localhost:5432": -1, "127.0.0.1:6379": 0},
			wantCode: ExitNegative, wantOut: []string{"closed", "nothing accepts connections on localhost:5432", "start db (docker compose up -d db)"},
		},
		{
			name: "service down without description", open: map[string]int{"localhost:5432": 0, "127.0.0.1:6379": -1},
			wantCode: ExitNegative, wantOut: []string{"start cache\n"},
		},
		{
			name: "json", open: allOpen, args: []string{"--json"},
			wantOut: []string{`"kind": "doctor"`, `"ok": true`, `"expect": "open"`, `"host": "127.0.0.1"`, `"description": "docker compose up -d db"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := doctorHarness(t, doctorConfig, tt.busy, tt.holders, tt.open)
			args := tt.args
			if len(args) == 0 {
				args = []string{"-o", "plain"}
			}
			code := h.run(t, append([]string{"doctor"}, args...)...)
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
		})
	}
}

func TestDoctorCheckError(t *testing.T) {
	t.Parallel()
	h := doctorHarness(t, doctorConfig, nil, nil, map[string]int{"localhost:5432": 0, "127.0.0.1:6379": 0})
	h.app.Ports.(*runFinder).checkErr = errors.New("lsof failed")
	if code := h.run(t, "doctor", "-o", "plain"); code != ExitNegative || !strings.Contains(h.stdout.String(), "error   lsof failed") {
		t.Errorf("exit = %d\n%s", code, h.stdout)
	}
}

func TestDoctorConfigErrors(t *testing.T) {
	t.Parallel()
	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		h := doctorHarness(t, "", nil, nil, nil)
		if code := h.run(t, "doctor"); code != ExitUsage || !strings.Contains(h.stderr.String(), "no .portcheck.yaml found") || !strings.Contains(h.stderr.String(), "/guide/doctor/") {
			t.Errorf("exit = %d\n%s", code, h.stderr)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		h := doctorHarness(t, "version: 7\n", nil, nil, nil)
		if code := h.run(t, "doctor"); code != ExitUsage || !strings.Contains(h.stderr.String(), "version must be 1") {
			t.Errorf("exit = %d\n%s", code, h.stderr)
		}
	})
	t.Run("explicit path", func(t *testing.T) {
		t.Parallel()
		h := doctorHarness(t, "", nil, nil, map[string]int{"localhost:5432": 0, "127.0.0.1:6379": 0})
		p := filepath.Join(t.TempDir(), "custom.yaml")
		if err := os.WriteFile(p, []byte(doctorConfig), 0o600); err != nil {
			t.Fatal(err)
		}
		if code := h.run(t, "doctor", "--config", p); code != ExitOK || !strings.Contains(h.stderr.String(), p) {
			t.Errorf("exit = %d\n%s", code, h.stderr)
		}
	})
	t.Run("workdir error", func(t *testing.T) {
		t.Parallel()
		h := newHarness()
		h.app.Workdir = func() (string, error) { return "", errors.New("cwd deleted") }
		if code := h.run(t, "doctor"); code != ExitRuntime || !strings.Contains(h.stderr.String(), "cwd deleted") {
			t.Errorf("exit = %d\n%s", code, h.stderr)
		}
	})
	t.Run("print error", func(t *testing.T) {
		t.Parallel()
		h := doctorHarness(t, doctorConfig, nil, nil, map[string]int{"localhost:5432": 0, "127.0.0.1:6379": 0})
		h.app.Stdout = failWriter{}
		if code := h.run(t, "doctor"); code != ExitRuntime {
			t.Errorf("exit = %d", code)
		}
	})
	t.Run("no args", func(t *testing.T) {
		t.Parallel()
		if code := newHarness().run(t, "doctor", "x"); code != ExitUsage {
			t.Errorf("exit = %d", code)
		}
	})
}

func TestRunPortByName(t *testing.T) {
	t.Parallel()
	h := doctorHarness(t, doctorConfig, nil, nil, nil)
	if code := h.run(t, "run", "-q", "--port", "WEB", "srv"); code != 0 {
		t.Fatalf("exit = %d\n%s", code, h.stderr)
	}
	if r := h.app.Runner.(*fakeRunner); !strings.Contains(strings.Join(r.got.Env, " "), "PORT=3000") {
		t.Errorf("env = %v", r.got.Env)
	}

	h = doctorHarness(t, doctorConfig, nil, nil, nil)
	if code := h.run(t, "run", "--port", "nope", "srv"); code != ExitUsage || !strings.Contains(h.stderr.String(), "unknown port name") || !strings.Contains(h.stderr.String(), "known: web, db, cache") {
		t.Errorf("exit = %d\n%s", code, h.stderr)
	}

	h = doctorHarness(t, "", nil, nil, nil)
	if code := h.run(t, "run", "--port", "web", "srv"); code != ExitUsage || !strings.Contains(h.stderr.String(), "no .portcheck.yaml found") {
		t.Errorf("exit = %d\n%s", code, h.stderr)
	}
}
