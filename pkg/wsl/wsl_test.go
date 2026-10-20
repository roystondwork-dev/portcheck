package wsl

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/google/go-cmp/cmp"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

func utf16le(s string, bom bool) []byte {
	var b []byte
	if bom {
		b = append(b, 0xff, 0xfe)
	}
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

// Captured from Ubuntu 24.04 (ss from iproute2) and a minimal Debian without ss.
const ubuntuOut = `LISTEN 0      511          0.0.0.0:3000      0.0.0.0:*    users:(("node",pid=1234,fd=19))
LISTEN 0      4096   127.0.0.53%lo:53        0.0.0.0:*
LISTEN 0      511             [::]:8080         [::]:*    users:(("nginx",pid=88,fd=6))
garbage
LISTEN 0 1 nocolon x
LISTEN 0 1 0.0.0.0:0 x
ESTAB 0 0 10.0.0.2:22 10.0.0.1:5000
---portcheck---
  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1
   1: 3500007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000   101        0 2345 1
   2: 0100007F:1F90 0100007F:C000 01 00000000:00000000 00:00000000 00000000  1000        0 0 1
  sl  local_address                         remote_address                        st tx_queue
   0: 00000000000000000000000000000000:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000
`

const debianOut = `---portcheck---
  sl  local_address rem_address   st
   0: 00000000:1538 00000000:0000 0A 0
   1: nocolon 00000000:0000 0A 0
   2: 00000000:ZZZZ 00000000:0000 0A 0
   3: 00000000:0000 00000000:0000 0A 0
`

type call struct {
	name string
	args []string
	env  []string
}

type fakeWSL struct {
	mu      sync.Mutex
	calls   []call
	list    []byte
	listErr error
	distros map[string]string
	failing map[string]bool
}

func (f *fakeWSL) run(_ context.Context, name string, args, env []string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, call{name, args, env})
	f.mu.Unlock()
	if args[0] == "--list" {
		return f.list, f.listErr
	}
	d := args[1]
	if f.failing[d] {
		return nil, errors.New("distro failed to start")
	}
	return []byte(f.distros[d]), nil
}

func newFake() *fakeWSL {
	return &fakeWSL{
		list:    utf16le("Ubuntu\r\nDebian\r\nBroken\r\n", true),
		distros: map[string]string{"Ubuntu": ubuntuOut, "Debian": debianOut},
		failing: map[string]bool{"Broken": true},
	}
}

func relay(port string, name string) inspect.Socket {
	return inspect.Socket{
		RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: netip.MustParseAddrPort(port), State: inspect.StateListen, PID: 4000},
		Process:   &inspect.ProcessInfo{PID: 4000, Name: name},
	}
}

func TestEnrich(t *testing.T) {
	t.Parallel()
	f := newFake()
	e := &Enricher{run: f.run, exe: "wsl.exe"}
	docker := &inspect.Container{Name: "web", Runtime: "docker"}
	udp := relay("127.0.0.1:5353", "wslrelay.exe")
	udp.Proto, udp.State = inspect.UDP, inspect.StateNone
	established := relay("127.0.0.1:3000", "wslrelay.exe")
	established.State = inspect.StateEstablished
	alreadyContainer := relay("127.0.0.1:8080", "wslrelay.exe")
	alreadyContainer.Container = docker

	socks := []inspect.Socket{
		relay("127.0.0.1:3000", "wslrelay.exe"),   // Ubuntu, ss knows the process
		relay("[::1]:8080", "WSLRelay.exe"),       // case-insensitive name, IPv6
		relay("127.0.0.1:5432", "wslservice.exe"), // Debian, /proc only (no ss)
		relay("127.0.0.1:53", "wslrelay.exe"),     // listening in Ubuntu without process info
		relay("127.0.0.1:9999", "wslrelay.exe"),   // no distro listens: untouched
		relay("127.0.0.1:3001", "node.exe"),       // not a relay: untouched
		udp, established, alreadyContainer,        // ignored
	}
	if err := e.Enrich(t.Context(), socks); err != nil {
		t.Fatal(err)
	}
	got := make([]*inspect.Container, len(socks))
	for i, s := range socks {
		got[i] = s.Container
	}
	want := []*inspect.Container{
		{Name: "Ubuntu", Runtime: "wsl", Process: "node", ProcessPID: 1234},
		{Name: "Ubuntu", Runtime: "wsl", Process: "nginx", ProcessPID: 88},
		{Name: "Debian", Runtime: "wsl"},
		{Name: "Ubuntu", Runtime: "wsl"},
		nil, nil, nil, nil, docker,
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}

	// Every call asks for UTF-8 output; distros are queried with the script.
	for _, c := range f.calls {
		if c.name != "wsl.exe" || !slices.Contains(c.env, "WSL_UTF8=1") {
			t.Errorf("call %v", c)
		}
		if c.args[0] == "-d" && (c.args[2] != "--exec" || c.args[5] != listenScript) {
			t.Errorf("distro call args = %v", c.args)
		}
	}
}

func TestEnrichNoRelayIsFree(t *testing.T) {
	t.Parallel()
	f := newFake()
	e := &Enricher{run: f.run, exe: "wsl.exe"}
	if err := e.Enrich(t.Context(), []inspect.Socket{relay("127.0.0.1:3000", "node.exe")}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 0 {
		t.Errorf("wsl.exe called %d times without relay sockets", len(f.calls))
	}
}

func TestEnrichListError(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.listErr = errors.New("WSL not installed")
	e := &Enricher{run: f.run, exe: "wsl.exe"}
	if err := e.Enrich(t.Context(), []inspect.Socket{relay("127.0.0.1:3000", "wslrelay.exe")}); err == nil || !strings.Contains(err.Error(), "list WSL distros") {
		t.Errorf("error = %v", err)
	}
}

func TestRunningDistrosEncodings(t *testing.T) {
	t.Parallel()
	for name, out := range map[string][]byte{
		"utf16 bom":    utf16le("Ubuntu\r\nDebian\r\n", true),
		"utf16 no bom": utf16le("Ubuntu\r\nDebian\r\n", false),
		"utf8":         []byte("Ubuntu\nDebian\n\n"),
		"utf8 bom":     []byte("\xef\xbb\xbfUbuntu\r\nDebian\r\n"),
	} {
		f := &fakeWSL{list: out}
		got, err := (&Enricher{run: f.run, exe: "wsl.exe"}).RunningDistros(t.Context())
		if err != nil || !slices.Equal(got, []string{"Ubuntu", "Debian"}) {
			t.Errorf("%s: %q, %v", name, got, err)
		}
	}
	f := &fakeWSL{list: nil}
	if got, err := (&Enricher{run: f.run, exe: "wsl.exe"}).RunningDistros(t.Context()); err != nil || len(got) != 0 {
		t.Errorf("no running distros: %q, %v", got, err)
	}
}

func TestListenersError(t *testing.T) {
	t.Parallel()
	f := newFake()
	if _, err := (&Enricher{run: f.run, exe: "wsl.exe"}).Listeners(t.Context(), "Broken"); err == nil || !strings.Contains(err.Error(), `"Broken"`) {
		t.Errorf("error = %v", err)
	}
}

func TestDecodeShortInput(t *testing.T) {
	t.Parallel()
	if decode([]byte("x")) != "x" || decode(nil) != "" {
		t.Error("short input must pass through")
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	e := New()
	if e.exe != "wsl.exe" || e.run == nil || e.EnrichTimeout() != 3*time.Second {
		t.Errorf("New = %+v", e)
	}
	if !slices.Contains(wslEnv(), "WSL_UTF8=1") {
		t.Error("wslEnv must request UTF-8")
	}
}

func TestExecRun(t *testing.T) {
	t.Parallel()
	// The test binary itself is a convenient command that exists on every OS.
	if _, err := execRun(t.Context(), os.Args[0], []string{"-test.run=^$"}, os.Environ()); err != nil {
		t.Errorf("execRun(test binary) = %v", err)
	}
	if _, err := execRun(t.Context(), "portcheck-no-such-wsl-exe", nil, nil); err == nil || !strings.Contains(err.Error(), "portcheck-no-such-wsl-exe") {
		t.Errorf("missing command error = %v", err)
	}
}
