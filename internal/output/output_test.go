package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/internal/buildinfo"
	"github.com/roystondwork-dev/portcheck/internal/testutil"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/scan"
)

var (
	node = &inspect.ProcessInfo{PID: 4242, Name: "node", User: "alice", Cmdline: "node server.js", Exe: "/usr/local/bin/node", CreateTime: time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)}
	pg   = &inspect.ProcessInfo{PID: 88, Name: "postgres"}

	sockets = []inspect.Socket{
		{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: netip.MustParseAddrPort("0.0.0.0:22"), State: inspect.StateListen}},
		{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: netip.MustParseAddrPort("127.0.0.1:3000"), State: inspect.StateListen, PID: 4242}, Process: node},
		{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: netip.MustParseAddrPort("127.0.0.1:3000"), Remote: netip.MustParseAddrPort("127.0.0.1:51234"), State: inspect.StateEstablished, PID: 4242}, Process: node},
		{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: netip.MustParseAddrPort("[::]:5432"), State: inspect.StateListen, PID: 88}, Process: pg},
		{RawSocket: inspect.RawSocket{Proto: inspect.UDP, Local: netip.MustParseAddrPort("0.0.0.0:5353"), State: inspect.StateNone, PID: 300}},
		{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: netip.MustParseAddrPort("10.0.0.2:50000"), Remote: netip.MustParseAddrPort("1.1.1.1:443"), State: inspect.StateTimeWait}},
	}

	statuses = []freeport.Status{
		{Port: 3000, Proto: inspect.TCP, Availability: freeport.InUse, Holders: sockets[1:2]},
		{Port: 3001, Proto: inspect.TCP, Availability: freeport.Free},
		{Port: 80, Proto: inspect.TCP, Availability: freeport.Denied},
		{Port: 22, Proto: inspect.TCP, Availability: freeport.InUse},
	}

	killResults = []KillResultJSON{
		{PID: 4242, Name: "node", Outcome: "terminated"},
		{PID: 77, Name: "java", Outcome: "killed"},
		{PID: 1, Name: "launchd", Outcome: "failed", Error: "refusing to kill protected process"},
		{PID: 99, Outcome: "already-exited"},
		{PID: 5, Name: "x", Outcome: "dry-run"},
	}

	scanResults = []scan.Result{
		{Target: scan.Target{Host: "db.internal", Addr: netip.MustParseAddr("10.0.0.5")}, Port: 5432, State: scan.Open, Service: "postgresql", Latency: 1234 * time.Microsecond},
		{Target: scan.Target{Host: "db.internal", Addr: netip.MustParseAddr("10.0.0.5")}, Port: 6379, State: scan.Closed, Service: "redis", Latency: 800 * time.Microsecond},
		{Target: scan.Target{Host: "10.0.0.6", Addr: netip.MustParseAddr("10.0.0.6")}, Port: 22, State: scan.Filtered, Service: "ssh", Latency: time.Second},
		{Target: scan.Target{Host: "10.0.0.7", Addr: netip.MustParseAddr("10.0.0.7")}, Port: 4, State: scan.Error, Latency: 2 * time.Millisecond, Err: errors.New("no route to host")},
	}
	scanSummary = scan.Summary{Hosts: 3, Ports: 4, Open: 1, Closed: 1, Filtered: 1, Errored: 1}

	version = buildinfo.Info{Version: "v0.1.0", Commit: "abc123", Date: "2026-10-20T00:00:00Z", GoVersion: "go1.27.3", Platform: "darwin/arm64"}
)

var colorEnv = []string{"CLICOLOR_FORCE=1", "TERM=xterm-256color"}

func docs() map[string]Document {
	return map[string]Document{
		"list":       ListDoc(&inspect.Result{Sockets: sockets, Partial: true}),
		"list-empty": ListDoc(&inspect.Result{}),
		"check":      CheckDoc(statuses),
		"free":       FreeDoc([]uint16{3001, 3002}),
		"free-empty": FreeDoc(nil),
		"kill":       KillDoc(killResults),
		"version":    VersionDoc(version),
		"scan":       ScanDoc(scanResults, scanSummary, 1500*time.Millisecond),
		"scan-empty": ScanDoc(nil, scan.Summary{}, 0),
		"wait": WaitDoc([]WaitResultJSON{
			{Target: "5432", Host: "localhost", Port: 5432, Condition: "open", Ready: true, WaitedMs: 1250, Attempts: 6},
			{Target: "db.internal:6379", Host: "db.internal", Port: 6379, Condition: "open", WaitedMs: 30000, Attempts: 120},
			{Target: "3000", Host: "localhost", Port: 3000, Condition: "free", Ready: true, Attempts: 1},
		}),
	}
}

func render(t *testing.T, d Document, f Format, opts Options) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := NewPrinter(&buf, f, opts).Print(d); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGolden(t *testing.T) {
	t.Parallel()
	for name, d := range docs() {
		for _, f := range Formats {
			t.Run(name+"/"+string(f), func(t *testing.T) {
				t.Parallel()
				testutil.Golden(t, name+"."+string(f), render(t, d, f, Options{}))
			})
		}
		t.Run(name+"/table-color", func(t *testing.T) {
			t.Parallel()
			testutil.Golden(t, name+".table-color", render(t, d, Table, Options{Environ: colorEnv}))
		})
	}
}

func TestNoColorWins(t *testing.T) {
	t.Parallel()
	d := CheckDoc(statuses)
	colored := render(t, d, Table, Options{Environ: colorEnv})
	if !bytes.Contains(colored, []byte("\x1b[")) {
		t.Fatal("expected ANSI escapes with CLICOLOR_FORCE")
	}
	plain := render(t, d, Table, Options{Environ: colorEnv, NoColor: true})
	if bytes.Contains(plain, []byte("\x1b[")) {
		t.Error("NoColor output contains ANSI escapes")
	}
	withNoColor := render(t, d, Table, Options{Environ: append([]string{"NO_COLOR=1"}, colorEnv...)})
	if bytes.Contains(withNoColor, []byte("\x1b[")) {
		t.Error("NO_COLOR output contains ANSI escapes")
	}
	emptyNoColor := render(t, d, Table, Options{Environ: append([]string{"NO_COLOR="}, colorEnv...)})
	if !bytes.Contains(emptyNoColor, []byte("\x1b[")) {
		t.Error("empty NO_COLOR must not disable color")
	}
	for _, f := range []Format{Plain, CSV, JSON} {
		if out := render(t, d, f, Options{Environ: colorEnv}); bytes.Contains(out, []byte("\x1b[")) {
			t.Errorf("%s output contains ANSI escapes", f)
		}
	}
}

func TestJSONEnvelope(t *testing.T) {
	t.Parallel()
	var env struct {
		SchemaVersion int             `json:"schemaVersion"`
		Kind          string          `json:"kind"`
		Data          json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(render(t, ListDoc(&inspect.Result{}), JSON, Options{}), &env); err != nil {
		t.Fatal(err)
	}
	if env.SchemaVersion != SchemaVersion || env.Kind != "list" {
		t.Errorf("envelope = %+v", env)
	}
	if string(env.Data) != `{"sockets":[],"partial":false}` && !strings.Contains(string(env.Data), `"sockets": []`) {
		t.Errorf("empty list must encode sockets as [], got %s", env.Data)
	}
	if out := render(t, FreeDoc(nil), JSON, Options{}); !bytes.Contains(out, []byte(`"ports": []`)) {
		t.Errorf("empty free must encode ports as [], got %s", out)
	}
	if out := render(t, ScanDoc(nil, scan.Summary{}, 0), JSON, Options{}); !bytes.Contains(out, []byte(`"results": []`)) {
		t.Errorf("empty scan must encode results as [], got %s", out)
	}
	if out := render(t, WaitDoc(nil), JSON, Options{}); !bytes.Contains(out, []byte(`"targets": []`)) {
		t.Errorf("empty wait must encode targets as [], got %s", out)
	}
	if out := render(t, KillDoc(nil), JSON, Options{}); !bytes.Contains(out, []byte(`"results": []`)) {
		t.Errorf("empty kill must encode results as [], got %s", out)
	}
}

func TestParseFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want Format
	}{{"table", Table}, {" JSON ", JSON}, {"csv", CSV}, {"Plain", Plain}}
	for _, tt := range tests {
		got, err := ParseFormat(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("ParseFormat(%q) = %q, %v", tt.in, got, err)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil || !strings.Contains(err.Error(), "yaml") {
		t.Errorf("ParseFormat(yaml) error = %v", err)
	}
}

func TestPrinterFormat(t *testing.T) {
	t.Parallel()
	if got := NewPrinter(&bytes.Buffer{}, CSV, Options{}).Format(); got != CSV {
		t.Errorf("Format() = %q", got)
	}
}

func TestCellHelpers(t *testing.T) {
	t.Parallel()
	if C("a") != (Cell{Text: "a"}) || T("b", ToneBad) != (Cell{Text: "b", Tone: ToneBad}) {
		t.Error("cell helpers broken")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestWriteErrors(t *testing.T) {
	t.Parallel()
	d := FreeDoc([]uint16{1})
	for _, f := range Formats {
		if err := NewPrinter(failWriter{}, f, Options{}).Print(d); err == nil {
			t.Errorf("%s: expected write error", f)
		}
	}
	if err := NewPrinter(failWriter{}, Plain, Options{}).Print(VersionDoc(version)); err == nil {
		t.Error("plain text: expected write error")
	}
	// Unencodable payloads surface as errors too.
	if err := NewPrinter(&bytes.Buffer{}, JSON, Options{}).Print(Document{Data: make(chan int)}); err == nil {
		t.Error("expected JSON encode error")
	}
}
