// Package wsl attributes ports forwarded from WSL (Windows Subsystem for
// Linux) to the distro and Linux process that listen on them.
//
// On Windows, a port bound inside a WSL 2 distro appears to belong to
// wslrelay.exe (NAT networking) or wslservice.exe (mirrored networking). All
// distros share one VM, so the relay doesn't reveal which distro is behind a
// port. Enricher asks each running distro for its listening TCP ports and
// labels matching sockets, e.g. "wsl:Ubuntu/node".
//
// Enricher only does work when such a relay socket is present, so it is free
// on systems without WSL.
package wsl

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

// Runtime is the inspect.Container runtime set for WSL ports.
const Runtime = "wsl"

// relayProcesses hold ports on behalf of WSL (compared case-insensitively).
var relayProcesses = []string{"wslrelay.exe", "wslservice.exe"}

// CommandRunner runs a command and returns its standard output.
type CommandRunner func(ctx context.Context, name string, args, env []string) ([]byte, error)

// Enricher implements inspect.Enricher for WSL.
type Enricher struct {
	run CommandRunner
	exe string
}

var (
	_ inspect.Enricher        = (*Enricher)(nil)
	_ inspect.EnrichTimeouter = (*Enricher)(nil)
)

// New returns an Enricher that runs wsl.exe.
func New() *Enricher { return &Enricher{run: execRun, exe: "wsl.exe"} }

// EnrichTimeout gives wsl.exe more time than the default enricher budget:
// each call starts a process in the WSL VM.
func (*Enricher) EnrichTimeout() time.Duration { return 3 * time.Second }

// Listener is a TCP port listening inside a distro.
type Listener struct {
	Port    uint16
	Process string // empty when unknown
	PID     int32  // 0 when unknown
}

// Enrich labels listening sockets owned by a WSL relay with the distro and
// Linux process behind them.
func (e *Enricher) Enrich(ctx context.Context, sockets []inspect.Socket) error {
	var targets []int
	for i, s := range sockets {
		if s.Container == nil && s.Proto == inspect.TCP && s.IsListening() && isRelay(s.Process) {
			targets = append(targets, i)
		}
	}
	if len(targets) == 0 {
		return nil
	}

	distros, err := e.RunningDistros(ctx)
	if err != nil {
		return err
	}
	listeners := e.listenersByDistro(ctx, distros)

	for _, i := range targets {
		port := sockets[i].Port()
		for _, d := range distros { // distro order is stable: wsl.exe lists the default first
			if l, ok := listeners[d][port]; ok {
				sockets[i].Container = &inspect.Container{Name: d, Runtime: Runtime, Process: l.Process, ProcessPID: l.PID}
				break
			}
		}
	}
	return nil
}

func isRelay(p *inspect.ProcessInfo) bool {
	return p != nil && slices.Contains(relayProcesses, strings.ToLower(p.Name))
}

// RunningDistros lists running WSL distros.
func (e *Enricher) RunningDistros(ctx context.Context) ([]string, error) {
	out, err := e.run(ctx, e.exe, []string{"--list", "--running", "--quiet"}, wslEnv())
	if err != nil {
		return nil, fmt.Errorf("list WSL distros: %w", err)
	}
	var distros []string
	for line := range strings.Lines(decode(out)) {
		if name := strings.TrimSpace(line); name != "" {
			distros = append(distros, name)
		}
	}
	return distros, nil
}

// listenersByDistro queries every distro concurrently. Distros that fail are
// skipped.
func (e *Enricher) listenersByDistro(ctx context.Context, distros []string) map[string]map[uint16]Listener {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out = make(map[string]map[uint16]Listener, len(distros))
	)
	for _, d := range distros {
		wg.Go(func() {
			ls, err := e.Listeners(ctx, d)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			out[d] = ls
		})
	}
	wg.Wait()
	return out
}

// listenScript prints listening sockets with process names (ss, iproute2) when
// available, then the kernel's TCP tables, which every distro has.
const (
	sectionMarker = "---portcheck---"
	listenScript  = "ss -Hltnp 2>/dev/null; echo " + sectionMarker + "; cat /proc/net/tcp /proc/net/tcp6 2>/dev/null"
)

// Listeners returns the listening TCP ports of distro, keyed by port.
func (e *Enricher) Listeners(ctx context.Context, distro string) (map[uint16]Listener, error) {
	out, err := e.run(ctx, e.exe, []string{"-d", distro, "--exec", "sh", "-c", listenScript}, wslEnv())
	if err != nil {
		return nil, fmt.Errorf("query WSL distro %q: %w", distro, err)
	}
	ssOut, procOut, _ := strings.Cut(string(out), sectionMarker)
	ls := parseProcNetTCP(procOut)
	for port, l := range parseSS(ssOut) { // ss adds process names where it can
		ls[port] = l
	}
	return ls, nil
}

var ssProcessRE = regexp.MustCompile(`\(\("([^"]+)",pid=(\d+)`)

// parseSS parses `ss -Hltnp` lines such as:
//
//	LISTEN 0 511 0.0.0.0:3000 0.0.0.0:* users:(("node",pid=1234,fd=19))
func parseSS(out string) map[uint16]Listener {
	ls := map[uint16]Listener{}
	for line := range strings.Lines(out) {
		f := strings.Fields(line)
		if len(f) < 4 || f[0] != "LISTEN" {
			continue
		}
		local := f[3]
		port, err := strconv.ParseUint(local[strings.LastIndex(local, ":")+1:], 10, 16)
		if err != nil || port == 0 {
			continue
		}
		l := Listener{Port: uint16(port)}
		if m := ssProcessRE.FindStringSubmatch(line); m != nil {
			pid, _ := strconv.ParseInt(m[2], 10, 32) // digits only, by the regexp
			l.Process, l.PID = m[1], int32(pid)
		}
		ls[l.Port] = l
	}
	return ls
}

// tcpListen is the kernel's state code for LISTEN in /proc/net/tcp.
const tcpListen = "0A"

// parseProcNetTCP parses /proc/net/tcp{,6}, e.g.
//
//	sl  local_address rem_address   st ...
//	 0: 00000000:0BB8 00000000:0000 0A ...
func parseProcNetTCP(out string) map[uint16]Listener {
	ls := map[uint16]Listener{}
	for line := range strings.Lines(out) {
		f := strings.Fields(line)
		if len(f) < 4 || f[3] != tcpListen {
			continue
		}
		_, hexPort, ok := strings.Cut(f[1], ":")
		if !ok {
			continue
		}
		port, err := strconv.ParseUint(hexPort, 16, 16)
		if err != nil || port == 0 {
			continue
		}
		ls[uint16(port)] = Listener{Port: uint16(port)}
	}
	return ls
}

// decode converts wsl.exe output to a string. Older WSL versions write
// UTF-16LE even when WSL_UTF8=1 is set.
func decode(b []byte) string {
	if len(b) >= 2 && looksUTF16LE(b) {
		u := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
		}
		return strings.TrimPrefix(string(utf16.Decode(u)), "\ufeff")
	}
	return string(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")))
}

// looksUTF16LE reports whether b has a UTF-16LE BOM or the NUL high bytes
// typical of UTF-16LE ASCII text.
func looksUTF16LE(b []byte) bool {
	if b[0] == 0xff && b[1] == 0xfe {
		return true
	}
	return b[1] == 0 && bytes.Count(b, []byte{0}) >= len(b)/4
}

// wslEnv asks wsl.exe for UTF-8 output (supported since WSL 0.64).
func wslEnv() []string { return append(os.Environ(), "WSL_UTF8=1") }

func execRun(ctx context.Context, name string, args, env []string) ([]byte, error) {
	// name is always wsl.exe (set in New); args are fixed or distro names reported by wsl.exe.
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // see above

	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}
