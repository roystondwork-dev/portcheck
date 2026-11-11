package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/monitor"
)

// fakeMonitors answers checks from a map of endpoint → error ("" = up).
type fakeMonitors struct {
	mu   sync.Mutex
	down map[string]string
}

func (f *fakeMonitors) check(_ context.Context, t monitor.Target) monitor.Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	reason := f.down[t.Endpoint()]
	return monitor.Result{Name: t.Name, Endpoint: t.Endpoint(), Up: reason == "", Error: reason, Status: 200, Latency: 12, At: testNow}
}

// fakeNotifier records notifications and signals each one.
type fakeNotifier struct {
	err  error
	sent chan string
}

func (n *fakeNotifier) Notify(_ context.Context, _, message string) error {
	n.sent <- message
	return n.err
}

// syncBuffer is a bytes.Buffer safe for a writer and a reader in different goroutines.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func monitorHarness(t *testing.T) (*harness, string, *fakeMonitors) {
	t.Helper()
	h := newHarness()
	dir := t.TempDir()
	h.app.Environ = []string{configDirEnv + "=" + dir}
	f := &fakeMonitors{down: map[string]string{}}
	h.app.MonitorCheck = f.check
	return h, filepath.Join(dir, monitor.FileName), f
}

func writeMonitors(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMonitorAddListRemove(t *testing.T) {
	t.Parallel()
	h, path, f := monitorHarness(t)
	f.down["localhost:5432"] = "connection refused"

	if code := h.run(t, "monitor"); code != 0 || !strings.Contains(h.stdout.String(), "No monitors yet") {
		t.Fatalf("empty list: %d %q", code, h.stdout.String())
	}
	h.stdout.Reset()
	if code := h.run(t, "monitor", "add", "https://example.com/health", "--every", "30s", "--contains", "ok"); code != 0 {
		t.Fatalf("add: %d %s", code, h.stderr.String())
	}
	if out := h.stdout.String(); !strings.Contains(out, "Added example.com/health: checks https://example.com/health every 30s.") || !strings.Contains(out, "It's up now (12 ms).") {
		t.Errorf("add output: %s", out)
	}
	if code := h.run(t, "monitor", "add", "5432", "--name", "postgres", "--every", "2m"); code != 0 {
		t.Fatalf("add port: %d", code)
	}
	if !strings.Contains(h.stderr.String(), "postgres is down right now: connection refused") {
		t.Errorf("stderr: %s", h.stderr.String())
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "contains: ok") || !strings.Contains(string(b), "address: localhost:5432") {
		t.Errorf("file: %s", b)
	}
	h.stdout.Reset()
	if code := h.run(t, "monitor", "list"); code != 0 {
		t.Fatal(code)
	}
	for _, want := range []string{"example.com/health", "30s", `HTTP 200-299, contains "ok"`, "postgres", "2m", "accepts connections"} {
		if !strings.Contains(h.stdout.String(), want) {
			t.Errorf("list missing %q:\n%s", want, h.stdout.String())
		}
	}
	h.stdout.Reset()
	if code := h.run(t, "monitor", "ls", "--json"); code != 0 || !strings.Contains(h.stdout.String(), `"kind": "monitors"`) {
		t.Errorf("list json: %s", h.stdout.String())
	}
	h.stdout.Reset()
	if code := h.run(t, "monitor", "rm", "postgres"); code != 0 || !strings.Contains(h.stdout.String(), "Removed postgres.") {
		t.Errorf("remove: %d %s", code, h.stdout.String())
	}
	if code := h.run(t, "monitor", "remove", "https://example.com/health/"); code != 0 {
		t.Errorf("remove by url: %d", code)
	}
	if code := h.run(t, "monitor", "remove", "nothing"); code != ExitUsage {
		t.Errorf("remove missing: %d", code)
	}
}

func TestMonitorErrors(t *testing.T) {
	t.Parallel()
	runCases(t, []runCase{
		{name: "no config dir", args: []string{"monitor", "list"}, wantCode: ExitRuntime, wantErr: []string{"find the configuration folder"}},
	})
	h, path, _ := monitorHarness(t)
	for _, args := range [][]string{
		{"monitor", "add", "ftp://x"},
		{"monitor", "add", "https://x/", "--every", "1s"},
	} {
		if code := h.run(t, args...); code != ExitUsage {
			t.Errorf("%v: code %d", args, code)
		}
	}
	if code := h.run(t, "monitor", "add", "https://x/"); code != 0 {
		t.Fatal(code)
	}
	if code := h.run(t, "monitor", "add", "https://x/"); code != ExitUsage || !strings.Contains(h.stderr.String(), "already exists") {
		t.Errorf("duplicate: %d %s", code, h.stderr.String())
	}
	writeMonitors(t, path, "version: 1\nmonitors:\n  - name: x\n")
	for _, args := range [][]string{{"monitor", "list"}, {"monitor", "add", "3000"}, {"monitor", "check"}, {"monitor", "remove", "x"}, {"monitor", "run"}} {
		h.stderr.Reset()
		if code := h.run(t, args...); code != ExitRuntime || !strings.Contains(h.stderr.String(), "fix the file or remove it") {
			t.Errorf("%v with a bad file: %d %s", args, code, h.stderr.String())
		}
	}
	// A folder where the file should be: saving fails.
	_ = os.Remove(path)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if code := h.run(t, "monitor", "add", "3000"); code != ExitRuntime {
		t.Errorf("unreadable file: %d", code)
	}
}

func TestMonitorSaveFails(t *testing.T) {
	t.Parallel()
	h, path, _ := monitorHarness(t)
	writeMonitors(t, path, "version: 1\nmonitors:\n  - {name: a, address: 'a:1'}\n")
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if os.WriteFile(filepath.Join(dir, "probe"), nil, 0o600) == nil {
		t.Skip("the folder is still writable (running as root?)")
	}
	for _, args := range [][]string{{"monitor", "add", "3000"}, {"monitor", "remove", "a"}} {
		if code := h.run(t, args...); code != ExitPermission {
			t.Errorf("%v in a read-only folder: %d", args, code)
		}
	}
}

func TestMonitorCheck(t *testing.T) {
	t.Parallel()
	h, path, f := monitorHarness(t)
	writeMonitors(t, path, "version: 1\nmonitors:\n  - {name: site, url: 'https://example.com/'}\n  - {name: db, address: 'db:5432'}\n")
	if code := h.run(t, "monitor", "check"); code != 0 || !strings.Contains(h.stdout.String(), "HTTP 200") {
		t.Errorf("all up: %d %s", code, h.stdout.String())
	}
	f.down["db:5432"] = "connection refused"
	h.stdout.Reset()
	if code := h.run(t, "monitor", "check"); code != ExitNegative || !strings.Contains(h.stdout.String(), "connection refused") {
		t.Errorf("one down: %d %s", code, h.stdout.String())
	}
	h.stdout.Reset()
	if code := h.run(t, "monitor", "check", "site", "--json"); code != 0 || !strings.Contains(h.stdout.String(), `"kind": "monitorCheck"`) || strings.Contains(h.stdout.String(), "db:5432") {
		t.Errorf("named: %d %s", code, h.stdout.String())
	}
	if code := h.run(t, "monitor", "check", "nope"); code != ExitUsage {
		t.Errorf("unknown name: %d", code)
	}
}

// runMonitor starts `monitor run` and returns a function that stops it and
// returns its exit code.
func runMonitor(t *testing.T, h *harness, args ...string) func() int {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	code := make(chan int, 1)
	go func() { code <- Execute(ctx, h.app, append([]string{"monitor", "run"}, args...)) }()
	return func() int {
		cancel()
		return <-code
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestMonitorRun(t *testing.T) {
	t.Parallel()
	h, path, f := monitorHarness(t)
	writeMonitors(t, path, "version: 1\nmonitors:\n  - {name: db, address: 'db:5432'}\n")
	f.down["db:5432"] = "connection refused"
	n := &fakeNotifier{sent: make(chan string, 8)}
	h.app.Notify = n
	stdout, stderr := &syncBuffer{}, &syncBuffer{}
	h.app.Stdout, h.app.Stderr = stdout, stderr
	stop := runMonitor(t, h, "--threshold", "1")
	if msg := <-n.sent; msg != "db is down: connection refused" {
		t.Errorf("notification = %q", msg)
	}
	if code := stop(); code != 0 {
		t.Errorf("exit code = %d", code)
	}
	if !strings.Contains(stdout.String(), "DOWN  db is down: connection refused") || !strings.Contains(stderr.String(), "Monitoring 1 service (Ctrl-C to stop)") {
		t.Errorf("stdout %q stderr %q", stdout.String(), stderr.String())
	}
}

func TestMonitorRunNotifyFailsAndReloads(t *testing.T) {
	t.Parallel()
	h, path, f := monitorHarness(t)
	f.down["a:1"], f.down["b:1"] = "refused", "refused"
	n := &fakeNotifier{sent: make(chan string, 8), err: errors.New("notify-send: not found")}
	h.app.Notify = n
	stdout, stderr := &syncBuffer{}, &syncBuffer{}
	h.app.Stdout, h.app.Stderr = stdout, stderr
	stop := runMonitor(t, h, "--threshold", "1", "--reload-every", "5ms")
	waitFor(t, "the empty-list message", func() bool { return strings.Contains(stderr.String(), "No monitors yet") })

	writeMonitors(t, path, "version: 1\nmonitors:\n  - {name: a, address: 'a:1'}\n")
	<-n.sent
	writeMonitors(t, path, "version: 1\nmonitors:\n  - {name: a, address: 'a:1'}\n  - {name: b, address: 'b:1'}\n")
	<-n.sent // b (a was already down)
	writeMonitors(t, path, "version: 1\nmonitors: [\n")
	waitFor(t, "the bad-file message", func() bool { return strings.Contains(stderr.String(), "still using the previous monitors") })
	if code := stop(); code != 0 {
		t.Errorf("exit code = %d", code)
	}
	errOut := stderr.String()
	if strings.Count(errOut, "notify-send: not found") != 1 || !strings.Contains(errOut, "Monitors changed; reloading.") {
		t.Errorf("stderr:\n%s", errOut)
	}
	if !strings.Contains(stdout.String(), "b is down: refused") {
		t.Errorf("stdout:\n%s", stdout.String())
	}
}

func TestMonitorRunNoNotify(t *testing.T) {
	t.Parallel()
	h, path, f := monitorHarness(t)
	writeMonitors(t, path, "version: 1\nmonitors:\n  - {name: db, address: 'db:5432'}\n")
	f.down["db:5432"] = "refused"
	n := &fakeNotifier{sent: make(chan string, 1)}
	h.app.Notify = n
	stdout := &syncBuffer{}
	h.app.Stdout, h.app.Stderr = stdout, &syncBuffer{}
	stop := runMonitor(t, h, "--threshold", "1", "--no-notify")
	waitFor(t, "the alert", func() bool { return strings.Contains(stdout.String(), "DOWN") })
	stop()
	if len(n.sent) != 0 {
		t.Error("notified despite --no-notify")
	}
}

func TestMonitorRunFlags(t *testing.T) {
	t.Parallel()
	h, _, _ := monitorHarness(t)
	for _, args := range [][]string{{"monitor", "run", "--threshold", "0"}, {"monitor", "run", "--reload-every", "0s"}} {
		if code := h.run(t, args...); code != ExitUsage {
			t.Errorf("%v: %d", args, code)
		}
	}
	h2 := newHarness()
	if code := h2.run(t, "monitor", "run"); code != ExitRuntime {
		t.Errorf("no config dir: %d", code)
	}
}
