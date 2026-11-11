package monitor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestParseTarget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		arg     string
		name    string
		url     string
		address string
	}{
		{"https://royston.in/ping", "royston.in/ping", "https://royston.in/ping", ""},
		{" http://localhost:3000/ ", "localhost:3000", "http://localhost:3000/", ""},
		{"db.internal:5432", "db.internal:5432", "", "db.internal:5432"},
		{"3000", "localhost:3000", "", "localhost:3000"},
		{"[::1]:8080", "[::1]:8080", "", "[::1]:8080"},
	} {
		got, err := ParseTarget(tc.arg)
		if err != nil || got.Name != tc.name || got.URL != tc.url || got.Address != tc.address {
			t.Errorf("ParseTarget(%q) = %+v, %v", tc.arg, got, err)
		}
	}
	for _, arg := range []string{"https://", "ftp://example.com", "nohost", ":80", "host:0", "host:http", "70000"} {
		if _, err := ParseTarget(arg); err == nil {
			t.Errorf("ParseTarget(%q) succeeded", arg)
		}
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	ok := Target{Name: "a", URL: "https://a.example/", Every: "30s", Timeout: "5s", Status: "200,204", Contains: "pong"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("Validate(ok) = %v", err)
	}
	for _, tc := range []struct {
		t    Target
		want string
	}{
		{Target{URL: "https://a/"}, "needs a name"},
		{Target{Name: "a", URL: "https://a/", Address: "a:1"}, "not both"},
		{Target{Name: "a"}, "set a url or an address"},
		{Target{Name: "a", URL: "http//a"}, "not an http or https URL"},
		{Target{Name: "a", URL: "https://"}, "not an http or https URL"},
		{Target{Name: "a", URL: "http://a b/"}, "not an http or https URL"},
		{Target{Name: "a", Address: "a:1", Status: "200"}, "only apply to URLs"},
		{Target{Name: "a", Address: "a:1", Contains: "x"}, "only apply to URLs"},
		{Target{Name: "a", Address: "a:1", Every: "soon"}, "every: \"soon\" is not a duration"},
		{Target{Name: "a", Address: "a:1", Every: "5s"}, "at least 10s"},
		{Target{Name: "a", Address: "a:1", Timeout: "x"}, "timeout: \"x\" is not a duration"},
		{Target{Name: "a", Address: "a:1", Timeout: "0s"}, "timeout must be positive"},
		{Target{Name: "a", Address: "a:1", Every: "10s", Timeout: "10s"}, "must be shorter"},
		{Target{Name: "a", URL: "https://a/", Status: "ok"}, "status \"ok\""},
		{Target{Name: "a", URL: "https://a/", Status: "99"}, "status"},
		{Target{Name: "a", URL: "https://a/", Status: "200-600"}, "status"},
		{Target{Name: "a", URL: "https://a/", Status: "300-200"}, "status"},
		{Target{Name: "a", URL: "https://a/", Status: "200-x"}, "status"},
	} {
		err := tc.t.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Validate(%+v) = %v, want %q", tc.t, err, tc.want)
		}
	}
}

func TestTargetHelpers(t *testing.T) {
	t.Parallel()
	u := Target{Name: "u", URL: "https://a/"}
	a := Target{Name: "a", Address: "a:1"}
	if u.Endpoint() != "https://a/" || a.Endpoint() != "a:1" {
		t.Errorf("Endpoint = %q, %q", u.Endpoint(), a.Endpoint())
	}
	if d, _ := a.Interval(); d != DefaultEvery {
		t.Errorf("Interval = %s", d)
	}
	if d, _ := a.TimeoutDuration(); d != DefaultTimeout {
		t.Errorf("Timeout = %s", d)
	}
	s, _ := Target{Status: "200, 300-399"}.Statuses()
	for code, want := range map[int]bool{200: true, 201: false, 300: true, 399: true, 404: false} {
		if s.Has(code) != want {
			t.Errorf("Has(%d) = %v", code, !want)
		}
	}
	if d, _ := u.Statuses(); !d.Has(204) || d.Has(301) {
		t.Errorf("default statuses = %v", d)
	}
}

func TestFileRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sub", FileName)
	f, err := Load(path)
	if err != nil || f.Version != 1 || len(f.Monitors) != 0 {
		t.Fatalf("Load(missing) = %+v, %v", f, err)
	}
	site, _ := ParseTarget("https://royston.in/ping")
	site.Every, site.Contains = "5m", "pong"
	db, _ := ParseTarget("db.internal:5432")
	if err := f.Add(site); err != nil {
		t.Fatal(err)
	}
	if err := f.Add(db); err != nil {
		t.Fatal(err)
	}
	if err := f.Add(site); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Add(duplicate) = %v", err)
	}
	if err := f.Add(Target{Name: "bad"}); err == nil {
		t.Error("Add(invalid) succeeded")
	}
	if err := Save(path, f); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), "# portcheck monitors") || !strings.Contains(string(b), "contains: pong") {
		t.Errorf("file = %s", b)
	}
	got, err := Load(path)
	if err != nil || len(got.Monitors) != 2 || got.Monitors[0] != site {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	if m, ok := got.Remove("https://royston.in/ping/"); !ok || m.Name != "royston.in/ping" {
		t.Errorf("Remove(url) = %+v, %v", m, ok)
	}
	if _, ok := got.Remove("db.internal:5432"); !ok || len(got.Monitors) != 0 {
		t.Errorf("Remove(name) left %+v", got.Monitors)
	}
	if _, ok := got.Remove("nothing"); ok {
		t.Error("Remove(missing) succeeded")
	}
}

func TestLoadErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if f, err := Load(write("v0.yaml", "monitors: []\n")); err != nil || f.Version != 1 {
		t.Errorf("Load(no version) = %+v, %v", f, err)
	}
	for name, tc := range map[string]struct{ text, want string }{
		"yaml":    {"monitors: [", "yaml"},
		"version": {"version: 2\n", "version 2 is not supported"},
		"invalid": {"version: 1\nmonitors:\n  - name: x\n", "set a url or an address"},
		"dup":     {"version: 1\nmonitors:\n  - {name: x, address: 'a:1'}\n  - {name: x, address: 'b:1'}\n", "two monitors are named"},
	} {
		if _, err := Load(write(name+".yaml", tc.text)); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Load(%s) = %v, want %q", name, err, tc.want)
		}
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "read ") {
		t.Errorf("Load(dir) = %v", err)
	}
}

// Not parallel: replaces the package's file hooks.
func TestSaveErrors(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(filepath.Join(blocker, "sub", FileName), &File{Version: 1}); err == nil || !strings.Contains(err.Error(), "create ") {
		t.Errorf("Save(under a file) = %v", err)
	}
	boom := errors.New("boom")
	path := filepath.Join(dir, FileName)
	for name, set := range map[string]func() func(){
		"marshal": func() func() {
			old := marshal
			marshal = func(any) ([]byte, error) { return nil, boom }
			return func() { marshal = old }
		},
		"createTemp": func() func() {
			old := createTemp
			createTemp = func(string, string) (*os.File, error) { return nil, boom }
			return func() { createTemp = old }
		},
		"writeFile": func() func() {
			old := writeFile
			writeFile = func(string, []byte, os.FileMode) error { return boom }
			return func() { writeFile = old }
		},
		"rename": func() func() {
			old := rename
			rename = func(string, string) error { return boom }
			return func() { rename = old }
		},
	} {
		restore := set()
		err := Save(path, &File{Version: 1})
		restore()
		if !errors.Is(err, boom) {
			t.Errorf("Save with failing %s = %v", name, err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".monitors-*")); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

func testChecker() *Checker {
	c := NewChecker("portcheck-test")
	return c
}

func TestCheckURL(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "portcheck-test" {
			w.WriteHeader(http.StatusTeapot)
		}
		_, _ = io.WriteString(w, "pong")
	})
	mux.HandleFunc("/down", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	mux.HandleFunc("/empty", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/ok", http.StatusFound) })
	mux.HandleFunc("/short", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, "po")
	})
	mux.HandleFunc("/slow", func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := testChecker()
	for _, tc := range []struct {
		target Target
		up     bool
		status int
		err    string
	}{
		{Target{Name: "ok", URL: srv.URL + "/ok"}, true, 200, ""},
		{Target{Name: "pong", URL: srv.URL + "/ok", Contains: "pong"}, true, 200, ""},
		{Target{Name: "moved", URL: srv.URL + "/moved", Contains: "pong"}, true, 200, ""},
		{Target{Name: "nopong", URL: srv.URL + "/ok", Contains: "ping"}, false, 200, `the response doesn't contain "ping"`},
		{Target{Name: "down", URL: srv.URL + "/down"}, false, 503, "HTTP 503 Service Unavailable"},
		{Target{Name: "204", URL: srv.URL + "/empty", Status: "204"}, true, 204, ""},
		{Target{Name: "short", URL: srv.URL + "/short", Contains: "pong"}, false, 200, "unexpected EOF"},
		{Target{Name: "slow", URL: srv.URL + "/slow", Timeout: "50ms"}, false, 0, "no answer within 50ms"},
		{Target{Name: "badurl", URL: "http://a b/"}, false, 0, "invalid character"},
	} {
		r := c.Check(t.Context(), tc.target)
		if r.Up != tc.up || r.Status != tc.status || !strings.Contains(r.Error, tc.err) || r.Name != tc.target.Name || r.Endpoint != tc.target.URL {
			t.Errorf("Check(%s) = %+v", tc.target.Name, r)
		}
		if r.Latency < 0 || r.At.IsZero() {
			t.Errorf("Check(%s) latency %v at %v", tc.target.Name, r.Latency, r.At)
		}
	}
}

func TestCheckTCP(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	c := testChecker()
	if r := c.Check(t.Context(), Target{Name: "db", Address: addr}); !r.Up || r.Error != "" || r.Endpoint != addr {
		t.Errorf("Check(open) = %+v", r)
	}
	_ = ln.Close()
	if r := c.Check(t.Context(), Target{Name: "db", Address: addr}); r.Up || r.Error != "connection refused" {
		t.Errorf("Check(closed) = %+v", r)
	}
	c.Dialer = blockingDialer{}
	if r := c.Check(t.Context(), Target{Name: "db", Address: addr, Timeout: "20ms"}); r.Up || r.Error != "no answer within 20ms" {
		t.Errorf("Check(silent) = %+v", r)
	}
}

type blockingDialer struct{}

func (blockingDialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

func TestCheckDNS(t *testing.T) {
	t.Parallel()
	c := testChecker()
	c.Client = &http.Client{Transport: failingTransport{&net.DNSError{Name: "nope.invalid", IsNotFound: true}}}
	if r := c.Check(t.Context(), Target{Name: "x", URL: "https://nope.invalid/"}); r.Up || r.Error != "host not found: nope.invalid" {
		t.Errorf("Check = %+v", r)
	}
}

func TestDescribe(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("dial: %w", syscall.ECONNRESET), "connection reset"},
		{fmt.Errorf("dial: %w", syscall.EHOSTUNREACH), "host unreachable"},
		{fmt.Errorf("dial: %w", syscall.ENETUNREACH), "host unreachable"},
		{fmt.Errorf("read: %w", os.ErrDeadlineExceeded), "timed out"},
		{context.DeadlineExceeded, "timed out"},
		{&net.DNSError{Name: "x", IsTimeout: true, Err: "i/o timeout"}, "i/o timeout"},
		{errors.New(`Get "https://a/": tls: failed to verify certificate: x509: expired`), "TLS certificate problem: expired"},
		{errors.New("plain"), "plain"},
	} {
		if got := describe(tc.err); got != tc.want {
			t.Errorf("describe(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func at(s int) time.Time { return time.Date(2026, 11, 11, 10, 0, s, 0, time.UTC) }

func TestTracker(t *testing.T) {
	t.Parallel()
	var tr Tracker
	ok := func(s int) Result { return Result{Name: "site", Endpoint: "https://s/", Up: true, At: at(s)} }
	bad := func(s int) Result { return Result{Name: "site", Endpoint: "https://s/", Error: "timed out", At: at(s)} }
	steps := []struct {
		r     Result
		event string
	}{
		{ok(0), ""},
		{bad(60), ""}, // one failure is not an outage
		{ok(120), ""},
		{bad(180), ""},
		{bad(240), "site is down: timed out"},
		{bad(300), ""}, // no repeats while down
		{ok(420), "site is back up after 4m"},
		{ok(480), ""},
	}
	for i, s := range steps {
		ev, fired := tr.Observe(s.r)
		got := ""
		if fired {
			got = ev.Message()
		}
		if got != s.event {
			t.Errorf("step %d: event %q, want %q", i, got, s.event)
		}
	}
	st := tr.States()["site"]
	if st.Status != Up || !st.Since.Equal(at(420)) || st.Fails != 0 || !st.Last.At.Equal(at(480)) {
		t.Errorf("state = %+v", st)
	}
}

func TestTrackerDownFromStart(t *testing.T) {
	t.Parallel()
	tr := Tracker{Threshold: 1}
	ev, fired := tr.Observe(Result{Name: "db", Error: "connection refused", At: at(0)})
	if !fired || ev.Status != Down || ev.Reason != "connection refused" {
		t.Errorf("event = %+v, %v", ev, fired)
	}
	if st := tr.States()["db"]; st.Status != Down || !st.Since.Equal(at(0)) {
		t.Errorf("state = %+v", st)
	}
	tr.Observe(Result{Name: "other", Up: true, At: at(1)})
	tr.Forget(map[string]bool{"other": true})
	if _, ok := tr.States()["db"]; ok || len(tr.States()) != 1 {
		t.Errorf("Forget left %v", tr.States())
	}
}

func TestRoundDuration(t *testing.T) {
	t.Parallel()
	for d, want := range map[time.Duration]string{
		1500 * time.Millisecond:           "2s",
		4*time.Minute + 20*time.Second:    "4m",
		3*time.Hour + 20*time.Minute:      "3h",
		59*time.Second + time.Millisecond: "59s",
	} {
		if got := roundDuration(d); got != want {
			t.Errorf("roundDuration(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestRunner(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var mu sync.Mutex
	var results []Result
	events := make(chan Event, 4)
	r := &Runner{
		// Up, down, down (event), up (event), …
		Check: func(_ context.Context, tg Target) Result {
			n := calls.Add(1)
			return Result{Name: tg.Name, Up: n == 1 || n >= 4, Error: "x", At: at(int(n))}
		},
		Tracker:  &Tracker{},
		OnResult: func(res Result) { mu.Lock(); results = append(results, res); mu.Unlock() },
		OnEvent:  func(e Event) { events <- e },
		Interval: func(Target) time.Duration { return time.Millisecond },
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		r.Run(ctx, []Target{{Name: "a", Address: "a:1"}})
		close(done)
	}()
	if e := <-events; e.Status != Down {
		t.Errorf("first event = %+v", e)
	}
	if e := <-events; e.Status != Up {
		t.Errorf("second event = %+v", e)
	}
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if len(results) < 4 {
		t.Errorf("results = %d", len(results))
	}
}

func TestRunnerCheckAll(t *testing.T) {
	t.Parallel()
	r := &Runner{
		Check:   func(_ context.Context, tg Target) Result { return Result{Name: tg.Name, Up: tg.Name == "a"} },
		Tracker: &Tracker{Threshold: 1},
	}
	got := r.CheckAll(t.Context(), []Target{{Name: "a"}, {Name: "b"}})
	if len(got) != 2 || !got[0].Up || got[1].Up {
		t.Errorf("CheckAll = %+v", got)
	}
	if r.Tracker.States()["b"].Status != Down {
		t.Errorf("states = %+v", r.Tracker.States())
	}
	// A check cut short by shutdown records nothing.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r.CheckAll(ctx, []Target{{Name: "c"}})
	if _, ok := r.Tracker.States()["c"]; ok {
		t.Error("cancelled check was recorded")
	}
	if d := r.interval(Target{Name: "x", Address: "x:1", Every: "2m"}); d != 2*time.Minute {
		t.Errorf("interval = %s", d)
	}
}

func TestSupervisor(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), FileName)
	if Stamp(path) != "missing" {
		t.Error("Stamp(missing) is not missing")
	}
	checked := make(chan string, 16)
	loads := make(chan error, 16)
	s := &Supervisor{
		Path: path,
		Runner: &Runner{
			Check: func(_ context.Context, tg Target) Result {
				checked <- tg.Name
				return Result{Name: tg.Name, Up: true}
			},
			Tracker:  &Tracker{},
			Interval: func(Target) time.Duration { return time.Hour },
		},
		Reload: 5 * time.Millisecond,
		OnLoad: func(_ *File, err error) { loads <- err },
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	if err := <-loads; err != nil || len(s.Targets()) != 0 {
		t.Fatalf("first load: %v, %v", err, s.Targets())
	}
	write := func(text string) {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("version: 1\nmonitors:\n  - {name: a, address: 'a:1'}\n")
	if err := <-loads; err != nil {
		t.Fatalf("second load: %v", err)
	}
	if name := <-checked; name != "a" {
		t.Errorf("checked %q", name)
	}
	write("version: 1\nmonitors: [\n")
	if err := <-loads; err == nil {
		t.Error("broken file loaded")
	}
	// The previous monitors keep running.
	if got := s.Targets(); len(got) != 1 || got[0].Name != "a" {
		t.Errorf("targets after a bad load = %+v", got)
	}
	cancel()
	<-done
}

func TestSupervisorDefaults(t *testing.T) {
	t.Parallel()
	s := &Supervisor{Path: filepath.Join(t.TempDir(), FileName), Runner: &Runner{Tracker: &Tracker{}}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s.Run(ctx) // returns at once; no OnLoad needed
}
