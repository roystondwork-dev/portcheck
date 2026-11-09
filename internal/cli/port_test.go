package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/internal/portreg"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// portEnv is a harness working inside a project "shop" (package.json, cwd in
// its web/ folder), with its own configuration folder.
type portEnv struct {
	h      *harness
	r      *fakeRunner
	f      *runFinder
	root   string // shop's root, symlinks resolved
	config string
}

func newPortEnv(t *testing.T) *portEnv {
	t.Helper()
	h, r, f := runHarness(nil)
	e := &portEnv{h: h, r: r, f: f, config: filepath.Join(t.TempDir(), "config")}
	e.root = e.project(t, "shop")
	e.cd(filepath.Join(e.root, "web"))
	h.app.ConfigDir = func() (string, error) { return e.config, nil }
	return e
}

// project creates a project folder and returns its real path.
func (e *portEnv) project(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(dir, "web"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func (e *portEnv) cd(dir string) { e.h.app.Workdir = func() (string, error) { return dir, nil } }

func (e *portEnv) run(t *testing.T, args ...string) int {
	t.Helper()
	e.h.stdout.Reset()
	e.h.stderr.Reset()
	return e.h.run(t, args...)
}

func (e *portEnv) registry(t *testing.T) *portreg.File {
	t.Helper()
	f, err := portreg.New(filepath.Join(e.config, "portcheck"), func(string, ...any) {}).Read()
	if err != nil {
		t.Fatal(err)
	}
	return f
}

var projectRange = portspec.MustParse(defaultProjectRange)

func TestPortAssignsAndRemembers(t *testing.T) {
	t.Parallel()
	e := newPortEnv(t)
	want := portreg.Preferred(e.root, projectRange)
	if code := e.run(t, "port"); code != ExitOK || e.h.stdout.String() != strconv.Itoa(int(want))+"\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, e.h.stdout, e.h.stderr)
	}
	if !strings.Contains(e.h.stderr.String(), "shop's port is now "+strconv.Itoa(int(want))) {
		t.Errorf("stderr = %q", e.h.stderr)
	}
	entry := e.registry(t).Projects[e.root]
	if entry.Port != want || entry.Name != "shop" || !entry.LastUsed.Equal(testNow) {
		t.Errorf("registry entry = %+v", entry)
	}
	// Again: the same port, without the message.
	if code := e.run(t, "port"); code != ExitOK || e.h.stdout.String() != strconv.Itoa(int(want))+"\n" || e.h.stderr.Len() != 0 {
		t.Fatalf("second run: exit %d, stdout %q, stderr %q", code, e.h.stdout, e.h.stderr)
	}
	// JSON says whether it was assigned now.
	if code := e.run(t, "port", "--json"); code != ExitOK {
		t.Fatal(e.h.stderr)
	}
	var got struct {
		Kind string          `json:"kind"`
		Data output.PortJSON `json:"data"`
	}
	if err := json.Unmarshal(e.h.stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "port" || got.Data.Port != want || got.Data.Assigned || got.Data.Path != e.root || got.Data.Project != "shop" {
		t.Errorf("json = %+v", got)
	}
}

func TestPortSkipsBusyAndAssigned(t *testing.T) {
	t.Parallel()
	e := newPortEnv(t)
	pref := portreg.Preferred(e.root, portspec.MustParse("4000-4002"))
	e.f.busy = map[uint16]freeport.Availability{pref: freeport.InUse}
	if code := e.run(t, "port", "--range", "4000-4002"); code != ExitOK {
		t.Fatal(e.h.stderr)
	}
	got, _ := strconv.Atoi(strings.TrimSpace(e.h.stdout.String()))
	if got == int(pref) || got < 4000 || got > 4002 {
		t.Errorf("assigned %d, want a free port in 4000-4002 other than the busy %d", got, pref)
	}
	// A second project skips the first one's port, even though it is free.
	other := e.project(t, "api")
	e.cd(other)
	e.f.busy = nil
	if code := e.run(t, "port", "--range", strconv.Itoa(got)+","+strconv.Itoa(int(pref))); code != ExitOK {
		t.Fatal(e.h.stderr)
	}
	if strings.TrimSpace(e.h.stdout.String()) != strconv.Itoa(int(pref)) {
		t.Errorf("api got %q, want %d (shop has %d)", e.h.stdout, pref, got)
	}
}

func TestPortNotesBusy(t *testing.T) {
	t.Parallel()
	e := newPortEnv(t)
	e.run(t, "port")
	p := e.registry(t).Projects[e.root].Port
	e.f.busy = map[uint16]freeport.Availability{p: freeport.InUse}
	if code := e.run(t, "port"); code != ExitOK || !strings.Contains(e.h.stderr.String(), "note: shop's port "+strconv.Itoa(int(p))+" is in use by node (pid 100)") {
		t.Errorf("exit %d, stderr %q", code, e.h.stderr)
	}
}

func TestPortErrors(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	for _, tc := range []struct {
		name     string
		args     []string
		setup    func(t *testing.T, e *portEnv)
		wantCode int
		wantErr  string
	}{
		{"combined modes", []string{"port", "--list", "--forget"}, nil, ExitUsage, "can't be combined"},
		{"force without set", []string{"port", "--force"}, nil, ExitUsage, "--force only applies to --set"},
		{"range with list", []string{"port", "--list", "--range", "4000-4999"}, nil, ExitUsage, "--range only applies"},
		{"set out of range", []string{"port", "--set", "0"}, nil, ExitUsage, "--set must be between"},
		{"bad range", []string{"port", "--range", "x"}, nil, ExitUsage, ""},
		{"no free port", []string{"port", "--range", "3000"}, func(_ *testing.T, e *portEnv) {
			e.f.busy = map[uint16]freeport.Availability{3000: freeport.InUse}
		}, ExitNegative, "can't assign a port to shop"},
		{"check fails", []string{"port"}, func(_ *testing.T, e *portEnv) { e.f.checkErr = errors.New("boom") }, ExitRuntime, "boom"},
		{"home is no project", []string{"port"}, func(_ *testing.T, e *portEnv) { e.cd(home) }, ExitUsage, "is not a project folder"},
		{"workdir fails", []string{"port"}, func(_ *testing.T, e *portEnv) {
			e.h.app.Workdir = func() (string, error) { return "", errors.New("gone") }
		}, ExitRuntime, "working directory: gone"},
		{"no config dir", []string{"port"}, func(_ *testing.T, e *portEnv) {
			e.h.app.ConfigDir = func() (string, error) { return "", errors.New("$HOME is not defined") }
		}, ExitRuntime, "find the configuration folder: $HOME is not defined (set PORTCHECK_CONFIG_DIR)"},
		{"config dir is a file", []string{"port"}, func(t *testing.T, e *portEnv) {
			if err := os.WriteFile(e.config, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}, ExitRuntime, "create"},
		{"forget without a port", []string{"port", "--forget"}, nil, ExitNegative, "shop has no port assigned."},
		{"forget, no project", []string{"port", "--forget"}, func(_ *testing.T, e *portEnv) { e.cd(home) }, ExitUsage, "is not a project folder"},
		{"set, no project", []string{"port", "--set", "3005"}, func(_ *testing.T, e *portEnv) { e.cd(home) }, ExitUsage, "is not a project folder"},
		{"list, unreadable", []string{"port", "--list"}, func(t *testing.T, e *portEnv) {
			if err := os.MkdirAll(filepath.Join(e.config, "portcheck", portreg.FileName), 0o700); err != nil {
				t.Fatal(err)
			}
		}, ExitRuntime, "read the port registry"},
		{"forget, unreadable", []string{"port", "--forget"}, func(t *testing.T, e *portEnv) {
			if err := os.MkdirAll(filepath.Join(e.config, "portcheck", portreg.FileName), 0o700); err != nil {
				t.Fatal(err)
			}
		}, ExitRuntime, "read the port registry"},
		{"set, unreadable", []string{"port", "--set", "3005"}, func(t *testing.T, e *portEnv) {
			if err := os.MkdirAll(filepath.Join(e.config, "portcheck", portreg.FileName), 0o700); err != nil {
				t.Fatal(err)
			}
		}, ExitRuntime, "read the port registry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newPortEnv(t)
			if tc.setup != nil {
				tc.setup(t, e)
			}
			if code := e.run(t, tc.args...); code != tc.wantCode || !strings.Contains(e.h.stderr.String(), tc.wantErr) {
				t.Errorf("exit %d (want %d), stderr %q (want %q)", code, tc.wantCode, e.h.stderr, tc.wantErr)
			}
		})
	}
}

func TestPortConfigDirEnv(t *testing.T) {
	t.Parallel()
	e := newPortEnv(t)
	dir := filepath.Join(t.TempDir(), "elsewhere")
	e.h.app.Environ = append(e.h.app.Environ, configDirEnv+"=/ignored", configDirEnv+"="+dir)
	if code := e.run(t, "port"); code != ExitOK {
		t.Fatal(e.h.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, portreg.FileName)); err != nil {
		t.Errorf("registry not in $%s: %v", configDirEnv, err)
	}
}

func TestPortListForgetSet(t *testing.T) {
	t.Parallel()
	e := newPortEnv(t)
	if code := e.run(t, "port", "--list"); code != ExitOK || !strings.Contains(e.h.stdout.String(), "No project has a port yet") {
		t.Fatalf("empty list: exit %d, %q", code, e.h.stdout)
	}
	shop := e.root
	if code := e.run(t, "port", "--set", "3005"); code != ExitOK || e.h.stdout.String() != "3005\n" {
		t.Fatalf("set: exit %d, %q %q", code, e.h.stdout, e.h.stderr)
	}
	api := e.project(t, "api")
	e.cd(api)
	if code := e.run(t, "port", "--set", "3005"); code != ExitNegative || !strings.Contains(e.h.stderr.String(), "port 3005 belongs to "+shop+"; pass --force to move it to api") {
		t.Fatalf("set taken: exit %d, %q", code, e.h.stderr)
	}
	if code := e.run(t, "port", "--set", "3001"); code != ExitOK {
		t.Fatal(e.h.stderr)
	}
	// A project whose folder is gone is marked in the list.
	gone := e.project(t, "old")
	e.cd(gone)
	if code := e.run(t, "port", "--set", "3009"); code != ExitOK {
		t.Fatal(e.h.stderr)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	if code := e.run(t, "port", "--list", "-o", "plain"); code != ExitOK {
		t.Fatal(e.h.stderr)
	}
	lines := strings.Split(strings.TrimSpace(e.h.stdout.String()), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[1], "3001  api") || !strings.HasPrefix(lines[2], "3005  shop") ||
		!strings.Contains(lines[3], gone+" (missing)") || !strings.Contains(lines[1], testNow.Format("2006-01-02")) {
		t.Errorf("list:\n%s", e.h.stdout)
	}
	// --force moves 3005 from shop to api.
	e.cd(api)
	if code := e.run(t, "port", "--set", "3005", "--force"); code != ExitOK {
		t.Fatal(e.h.stderr)
	}
	reg := e.registry(t)
	if _, ok := reg.Projects[shop]; ok || reg.Projects[api].Port != 3005 {
		t.Errorf("after --force: %+v", reg.Projects)
	}
	// Forget it.
	if code := e.run(t, "port", "--forget"); code != ExitOK || !strings.Contains(e.h.stdout.String(), "Forgot port 3005 of api") {
		t.Fatalf("forget: exit %d, %q", code, e.h.stdout)
	}
	if _, ok := e.registry(t).Projects[api]; ok {
		t.Error("api still has a port")
	}
}

func TestRunPortAuto(t *testing.T) {
	t.Parallel()
	t.Run("free", func(t *testing.T) {
		t.Parallel()
		e := newPortEnv(t)
		want := portreg.Preferred(e.root, projectRange)
		if code := e.run(t, "run", "--port", "auto", "--", "srv", "{port}"); code != ExitOK {
			t.Fatal(e.h.stderr)
		}
		if strings.Join(e.r.got.Args, " ") != strconv.Itoa(int(want)) || !strings.Contains(e.h.stderr.String(), "shop's port is now") {
			t.Errorf("args %v, stderr %q", e.r.got.Args, e.h.stderr)
		}
	})
	t.Run("busy, then --next for this run only", func(t *testing.T) {
		t.Parallel()
		e := newPortEnv(t)
		e.run(t, "port")
		p := e.registry(t).Projects[e.root].Port
		e.f.busy = map[uint16]freeport.Availability{p: freeport.InUse}
		if code := e.run(t, "run", "--port", "auto", "--", "srv"); code != ExitNegative || !strings.Contains(e.h.stderr.String(), "is in use by node (pid 100)") {
			t.Fatalf("exit %d, %q", code, e.h.stderr)
		}
		if code := e.run(t, "run", "--port", "auto", "--next", "--", "srv", "{port}"); code != ExitOK {
			t.Fatal(e.h.stderr)
		}
		if got := e.r.got.Args[0]; got == strconv.Itoa(int(p)) {
			t.Errorf("--next ran on the busy port %s", got)
		}
		if e.registry(t).Projects[e.root].Port != p {
			t.Error("--next changed the assignment")
		}
	})
	t.Run("explicit range", func(t *testing.T) {
		t.Parallel()
		e := newPortEnv(t)
		if code := e.run(t, "run", "--port", "auto", "--range", "5000", "--", "srv", "{port}"); code != ExitOK || e.r.got.Args[0] != "5000" {
			t.Fatalf("exit %d, args %v, %q", code, e.r.got.Args, e.h.stderr)
		}
	})
	t.Run("bad range", func(t *testing.T) {
		t.Parallel()
		e := newPortEnv(t)
		if code := e.run(t, "run", "--port", "auto", "--range", "x", "--", "srv"); code != ExitUsage {
			t.Errorf("exit %d", code)
		}
	})
	t.Run("no config dir", func(t *testing.T) {
		t.Parallel()
		e := newPortEnv(t)
		e.h.app.ConfigDir = func() (string, error) { return "", errors.New("nope") }
		if code := e.run(t, "run", "--port", "auto", "--", "srv"); code != ExitRuntime {
			t.Errorf("exit %d", code)
		}
	})
	t.Run("not in a project", func(t *testing.T) {
		t.Parallel()
		e := newPortEnv(t)
		e.h.app.Workdir = func() (string, error) { return "", errors.New("gone") }
		if code := e.run(t, "run", "--port", "auto", "--", "srv"); code != ExitRuntime {
			t.Errorf("exit %d", code)
		}
	})
}

func TestPortCorruptRegistry(t *testing.T) {
	t.Parallel()
	e := newPortEnv(t)
	dir := filepath.Join(e.config, "portcheck")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, portreg.FileName), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := e.run(t, "port"); code != ExitOK || !strings.Contains(e.h.stderr.String(), "is unreadable") || !strings.Contains(e.h.stderr.String(), "starting a new one") {
		t.Errorf("exit %d, stderr %q", code, e.h.stderr)
	}
}

func TestEnvValue(t *testing.T) {
	t.Parallel()
	env := []string{"A=1", "B=2", "A=3", "C"}
	if envValue(env, "A") != "3" || envValue(env, "B") != "2" || envValue(env, "C") != "" || envValue(env, "D") != "" {
		t.Error("envValue")
	}
}
