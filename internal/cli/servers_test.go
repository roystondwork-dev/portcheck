package cli

import (
	"encoding/json"
	"errors"
	"os/user"
	"strings"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

// serversHarness serves a dev server (shop, 2 days old, two ports), a newer
// one (api), one with no start time (docs), an app outside any project
// (ControlCenter), another user's server and a container.
func serversHarness() *harness {
	h := newHarness()
	listen := func(addr string, pid int32) inspect.RawSocket {
		return inspect.RawSocket{Proto: inspect.TCP, Local: ap(addr), State: inspect.StateListen, PID: pid}
	}
	h.provider.SocketList = []inspect.RawSocket{
		listen("127.0.0.1:5173", 10), listen("[::1]:5173", 10), listen("127.0.0.1:24678", 10),
		listen("127.0.0.1:8000", 20),
		listen("127.0.0.1:4000", 30),
		listen("0.0.0.0:5000", 40),
		listen("0.0.0.0:3000", 50),
	}
	h.provider.Processes = map[int32]*inspect.ProcessInfo{
		10: {PID: 10, Name: "node", User: "alice", Project: "shop", Cwd: "/home/alice/shop", CreateTime: testNow.Add(-50 * time.Hour),
			Ancestors: []inspect.ProcessRef{{PID: 9, Name: "npm"}, {PID: 8, Name: "zsh"}}},
		20: {PID: 20, Name: "python3", User: "alice", Project: "api", CreateTime: testNow.Add(-90 * time.Minute)},
		30: {PID: 30, Name: "hugo", User: "alice", Project: "docs"},
		40: {PID: 40, Name: "ControlCenter", User: "alice", Cwd: "/", CreateTime: testNow.Add(-72 * time.Hour)},
		50: {PID: 50, Name: "node", User: "bob", Project: "bobs", CreateTime: testNow.Add(-time.Hour)},
	}
	h.killer.names = map[int32]string{10: "node", 20: "python3", 30: "hugo"}
	return h
}

func TestServers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		args     []string
		setup    func(h *harness)
		wantCode int
		wantOut  []string
		notOut   []string
		wantErr  []string
	}{
		{name: "project servers, oldest first", args: []string{"servers"},
			wantOut: []string{"PID  PROCESS  PROJECT  PORTS       UPTIME\n10   node     shop     5173,24678  2d 2h\n20   python3  api      8000        1h 30m\n30   hugo     docs     4000        -\n"},
			notOut:  []string{"ControlCenter", "bobs"}},
		{name: "all", args: []string{"servers", "--all"}, wantOut: []string{"40   ControlCenter  -        5000"}, notOut: []string{"bobs"}},
		{name: "older than", args: []string{"servers", "--older-than", "1d"}, wantOut: []string{"shop"}, notOut: []string{"api", "docs"}},
		{name: "older than minutes", args: []string{"servers", "--older-than", "60m"}, wantOut: []string{"shop", "api"}, notOut: []string{"docs"}},
		{name: "none", args: []string{"servers", "--older-than", "30d"}, wantCode: ExitNegative, wantOut: []string{"No servers of yours are running in a project folder."}},
		{name: "windows user names", args: []string{"servers"}, setup: func(h *harness) {
			h.app.CurrentUser = func() (string, error) { return `PC\Alice`, nil }
			h.provider.Processes[10].User = `PC\alice`
		}, wantOut: []string{"shop"}},
		{name: "bad age", args: []string{"servers", "--older-than", "soon"}, wantCode: ExitUsage, wantErr: []string{`invalid --older-than "soon"`}},
		{name: "yes without stop", args: []string{"servers", "--yes"}, wantCode: ExitUsage, wantErr: []string{"only apply with --stop"}},
		{name: "bad grace", args: []string{"servers", "--grace", "0s"}, wantCode: ExitUsage},
		{name: "user unknown", args: []string{"servers"}, setup: func(h *harness) {
			h.app.CurrentUser = func() (string, error) { return "", errors.New("no passwd entry") }
		}, wantCode: ExitRuntime, wantErr: []string{"current user: no passwd entry"}},
		{name: "list error", args: []string{"servers"}, setup: func(h *harness) { h.provider.SocketsErr = errors.New("boom") }, wantCode: ExitRuntime},
		{name: "print error", args: []string{"servers"}, setup: func(h *harness) { h.app.Stdout = failWriter{} }, wantCode: ExitRuntime},
		{name: "stop", args: []string{"servers", "--older-than", "1h", "--stop"}, setup: func(h *harness) {
			h.app.StdinTTY, h.app.Stdin = true, strings.NewReader("y\n")
		}, wantErr: []string{
			"PID 10  node  (port 5173, 24678), started by npm (9) ← zsh (8), in shop\n",
			"PID 20  python3  (port 8000), in api\n",
		}, wantOut: []string{"10   node     terminated", "20   python3  terminated"}},
		{name: "stop dry run", args: []string{"servers", "--stop", "--dry-run"}, wantOut: []string{"dry-run"}},
		{name: "stop nothing", args: []string{"servers", "--older-than", "30d", "--stop"}, wantCode: ExitNegative, wantErr: []string{"No servers to stop."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := serversHarness()
			if tc.setup != nil {
				tc.setup(h)
			}
			if code := h.run(t, tc.args...); code != tc.wantCode {
				t.Fatalf("exit = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tc.wantCode, h.stdout, h.stderr)
			}
			for _, w := range tc.wantOut {
				if !strings.Contains(h.stdout.String(), w) {
					t.Errorf("stdout lacks %q:\n%s", w, h.stdout)
				}
			}
			for _, w := range tc.notOut {
				if strings.Contains(h.stdout.String(), w) {
					t.Errorf("stdout has %q:\n%s", w, h.stdout)
				}
			}
			for _, w := range tc.wantErr {
				if !strings.Contains(h.stderr.String(), w) {
					t.Errorf("stderr lacks %q:\n%s", w, h.stderr)
				}
			}
		})
	}
}

func TestServersJSON(t *testing.T) {
	t.Parallel()
	h := serversHarness()
	if code := h.run(t, "servers", "--json"); code != ExitOK {
		t.Fatalf("exit = %d: %s", code, h.stderr)
	}
	var got struct {
		Kind string             `json:"kind"`
		Data output.ServersJSON `json:"data"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "servers" || len(got.Data.Servers) != 3 {
		t.Fatalf("got %+v", got)
	}
	shop := got.Data.Servers[0]
	if shop.Project != "shop" || shop.Cwd != "/home/alice/shop" || shop.UptimeSeconds != 50*3600 || len(shop.Ports) != 2 {
		t.Errorf("shop = %+v", shop)
	}
	if docs := got.Data.Servers[2]; docs.StartTime != nil || docs.UptimeSeconds != 0 {
		t.Errorf("docs = %+v, want no start time or uptime", docs)
	}
}

func TestCollectServersSkipsContainers(t *testing.T) {
	t.Parallel()
	proc := &inspect.ProcessInfo{PID: 7, Name: "com.docker.backend", User: "alice", Project: "shop"}
	socks := []inspect.Socket{
		{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("0.0.0.0:8080"), State: inspect.StateListen, PID: 7}, Process: proc,
			Container: &inspect.Container{Name: "web", Runtime: "docker"}},
		{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("0.0.0.0:22"), State: inspect.StateListen}},
	}
	if got := collectServers(socks, "alice", true, testNow, 0); len(got) != 0 {
		t.Errorf("collectServers = %+v, want none (a container and a hidden owner)", got)
	}
}

func TestKillShowsProject(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.provider.Processes[100].Project = "shop"
	h.app.StdinTTY, h.app.Stdin = true, strings.NewReader("n\n")
	h.run(t, "kill", "3000")
	if want := "PID 100  node  (port 3000), started by npm (90) ← zsh (80), in shop\n"; !strings.Contains(h.stderr.String(), want) {
		t.Errorf("stderr lacks %q:\n%s", want, h.stderr)
	}
}

func TestSameUser(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		owner, me string
		want      bool
	}{
		{"alice", "alice", true},
		{"alice", "bob", false},
		{"", "alice", false},
		{"alice", "", false},
		{`PC\alice`, `PC\Alice`, true},
		{`PC\alice`, "alice", true},
		{`PC\bob`, `PC\alice`, false},
	} {
		if got := sameUser(tc.owner, tc.me); got != tc.want {
			t.Errorf("sameUser(%q, %q) = %v, want %v", tc.owner, tc.me, got, tc.want)
		}
	}
}

func TestParseAge(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]time.Duration{"": 0, "2d": 48 * time.Hour, "90m": 90 * time.Minute, "1h30m": 90 * time.Minute} {
		if got, err := parseAge(in); err != nil || got != want {
			t.Errorf("parseAge(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"0d", "-1d", "xd", "0s", "-5m", "soon"} {
		if _, err := parseAge(in); err == nil {
			t.Errorf("parseAge(%q) succeeded, want an error", in)
		}
	}
}

func TestCurrentUser(t *testing.T) {
	t.Parallel()
	if name, err := currentUser(); err != nil || name == "" {
		t.Errorf("currentUser() = %q, %v", name, err)
	}
	if _, err := userName(func() (*user.User, error) { return nil, errors.New("no passwd entry") }); err == nil {
		t.Error("userName succeeded despite a failed lookup")
	}
}
