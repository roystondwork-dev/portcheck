package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roystondwork-dev/portcheck/internal/cmdports"
	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/internal/testutil"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

// held3000 is port 3000 held by node (pid 100), started by npm ← zsh, in shop.
func held3000() freeport.Status {
	return freeport.Status{Port: 3000, Proto: inspect.TCP, Availability: freeport.InUse, Holders: []inspect.Socket{{
		RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("127.0.0.1:3000"), State: inspect.StateListen, PID: 100},
		Process: &inspect.ProcessInfo{PID: 100, Name: "node", Project: "shop",
			Ancestors: []inspect.ProcessRef{{PID: 90, Name: "npm"}, {PID: 80, Name: "zsh"}}},
	}}}
}

func TestExplain(t *testing.T) {
	t.Parallel()
	free := freeport.Status{Port: 3000, Proto: inspect.TCP, Availability: freeport.Free}
	runCases(t, []runCase{
		{name: "held explicit port", args: []string{"explain", "--", "vite --port 3000"},
			setup: func(h *harness) { h.finder.statuses = []freeport.Status{held3000()} },
			wantOut: []string{
				"portcheck: port 3000 is held by node (pid 100), started by npm (90) ← zsh (80), in shop\n",
				"portcheck: stop it: portcheck kill 3000  ·  or use a free port: portcheck run --port 3000 --next -- vite --port {port}\n",
			}},
		{name: "held default port: no run hint", args: []string{"explain", "--exit", "1", "--", "next", "dev"},
			setup:   func(h *harness) { h.finder.statuses = []freeport.Status{held3000()} },
			wantOut: []string{"portcheck: stop it: portcheck kill 3000\n"}, notOut: []string{"portcheck run"}},
		{name: "free port prints nothing", args: []string{"explain", "--", "vite --port 3000"},
			setup: func(h *harness) { h.finder.statuses = []freeport.Status{free} }, notOut: []string{"portcheck"}},
		{name: "no port prints nothing", args: []string{"explain", "--", "false"},
			setup: func(h *harness) { h.finder.statuses = []freeport.Status{held3000()} }, notOut: []string{"portcheck"}},
		{name: "interrupted", args: []string{"explain", "--exit", "130", "--", "vite --port 3000"},
			setup: func(h *harness) { h.finder.statuses = []freeport.Status{held3000()} }, notOut: []string{"portcheck"}},
		{name: "portcheck itself", args: []string{"explain", "--", "portcheck kill 3000"},
			setup: func(h *harness) { h.finder.statuses = []freeport.Status{held3000()} }, notOut: []string{"portcheck"}},
		{name: "check error prints nothing", args: []string{"explain", "--", "vite --port 3000"},
			setup: func(h *harness) { h.finder.checkErr = errors.New("boom") }, notOut: []string{"portcheck"}, notErr: []string{"boom"}},
		{name: "print error is quiet", args: []string{"explain", "--", "vite --port 3000"},
			setup: func(h *harness) {
				h.finder.statuses = []freeport.Status{held3000()}
				h.app.Stdout = failWriter{}
			}},
		{name: "json when nothing found", args: []string{"explain", "--json", "--", "false"},
			wantOut: []string{`"kind": "explain"`, `"ports": []`, `"inUse": []`}},
		{name: "usage", args: []string{"explain"}, wantCode: ExitUsage},
	})
}

func TestExplainJSON(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.finder.statuses = []freeport.Status{held3000()}
	if code := h.run(t, "explain", "-o", "json", "--", "PORT=3000 node server.js"); code != ExitOK {
		t.Fatalf("exit = %d: %s", code, h.stderr)
	}
	var got struct {
		Kind string             `json:"kind"`
		Data output.ExplainJSON `json:"data"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "explain" || got.Data.Command != "PORT=3000 node server.js" || got.Data.ExitCode != 1 ||
		len(got.Data.Ports) != 1 || got.Data.Ports[0] != (output.CandidateJSON{Port: 3000, Source: "explicit"}) ||
		len(got.Data.InUse) != 1 || got.Data.InUse[0].Holders[0].PID != 100 {
		t.Errorf("got %+v", got)
	}
}

func TestExplainPackageScripts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"vite --port 3000"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newHarness()
	h.app.Workdir = func() (string, error) { return dir, nil }
	h.finder.statuses = []freeport.Status{held3000()}
	h.run(t, "explain", "--", "npm run dev")
	if !strings.Contains(h.stdout.String(), "port 3000 is held by node") {
		t.Errorf("stdout:\n%s", h.stdout)
	}
	if strings.Contains(h.stdout.String(), "portcheck run") {
		t.Errorf("the port isn't on the command line itself, so no run hint:\n%s", h.stdout)
	}
}

func TestReadScripts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, tc := range []struct {
		name    string
		workdir func() (string, error)
		file    string
	}{
		{"no workdir", func() (string, error) { return "", errors.New("gone") }, ""},
		{"no package.json", func() (string, error) { return dir, nil }, ""},
		{"bad json", func() (string, error) { return dir, nil }, "{"},
	} {
		if tc.file != "" {
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(tc.file), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		c := &cli{app: &App{Workdir: tc.workdir}}
		if got := c.readScripts(); got != nil {
			t.Errorf("%s: readScripts = %v, want nil", tc.name, got)
		}
	}
}

func TestRunHint(t *testing.T) {
	t.Parallel()
	exp := []cmdports.Candidate{{Port: 3000, Source: cmdports.Explicit}}
	for _, tc := range []struct {
		line  string
		cands []cmdports.Candidate
		want  string
	}{
		{"vite --port 3000", exp, "portcheck run --port 3000 --next -- vite --port {port}"},
		{"vite --port=3000 --open", exp, "portcheck run --port 3000 --next -- vite --port={port} --open"},
		{"python3 -m http.server 3000", exp, "portcheck run --port 3000 --next -- python3 -m http.server {port}"},
		{"PORT=3000 node server.js", exp, ""},
		{"next dev", []cmdports.Candidate{{Port: 3000, Source: cmdports.Default}}, ""},
		{"npm run dev", exp, ""}, // the port came from package.json
		{"vite --port 30001", exp, ""},
	} {
		if got := runHint(tc.line, 3000, tc.cands); got != tc.want {
			t.Errorf("runHint(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}

func TestIsPortcheck(t *testing.T) {
	t.Parallel()
	for line, want := range map[string]bool{
		"portcheck kill 3000":        true,
		"  /usr/local/bin/portcheck": true,
		"portcheck.exe list":         true,
		"npx portcheck-cli 3000":     false,
		"portcheck-cli 3000":         true,
		"vite":                       false,
	} {
		if got := isPortcheck(line); got != want {
			t.Errorf("isPortcheck(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestInit(t *testing.T) {
	t.Parallel()
	for _, shell := range []string{"zsh", "bash", "fish", "powershell"} {
		h := newHarness()
		h.app.Executable = func() (string, error) { return "/opt/it's here/portcheck", nil }
		if code := h.run(t, "init", strings.ToUpper(shell)); code != ExitOK {
			t.Fatalf("init %s: exit %d: %s", shell, code, h.stderr)
		}
		testutil.Golden(t, "init."+shell, h.stdout.Bytes())
	}
	runCases(t, []runCase{
		{name: "unknown shell", args: []string{"init", "tcsh"}, wantCode: ExitUsage, wantErr: []string{`unknown shell "tcsh"`}},
		{name: "no shell", args: []string{"init"}, wantCode: ExitUsage},
		{name: "write error", args: []string{"init", "zsh"}, setup: func(h *harness) { h.app.Stdout = failWriter{} }, wantCode: ExitRuntime},
	})
}

func TestHookExecutable(t *testing.T) {
	t.Parallel()
	bin := filepath.Join(t.TempDir(), "portcheck")
	if err := os.WriteFile(bin, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := bin + "-link"
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		exe      func() (string, error)
		lookPath func(string) (string, error)
		want     string
	}{
		{"on PATH: by name", func() (string, error) { return bin, nil }, func(string) (string, error) { return link, nil }, "portcheck"},
		{"another portcheck on PATH", func() (string, error) { return bin, nil }, func(string) (string, error) { return "/nonexistent/portcheck", nil }, bin},
		{"not on PATH", func() (string, error) { return bin, nil }, func(string) (string, error) { return "", errors.New("not found") }, bin},
		{"no executable", func() (string, error) { return "", errors.New("unknown") }, nil, "portcheck"},
	} {
		c := &cli{app: &App{Executable: tc.exe, LookPath: tc.lookPath}}
		if got := c.hookExecutable(); got != tc.want {
			t.Errorf("%s: hookExecutable = %q, want %q", tc.name, got, tc.want)
		}
	}
}
