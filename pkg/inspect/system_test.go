package inspect

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"syscall"
	"testing"
	"time"

	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

type fakeHandle struct {
	name, user, cmdline, exe string
	createMS                 int64
	nameErr, otherErr        error
	ppid                     int32
	ppidErr                  error
}

func (h fakeHandle) PpidWithContext(context.Context) (int32, error) { return h.ppid, h.ppidErr }

func (h fakeHandle) NameWithContext(context.Context) (string, error) { return h.name, h.nameErr }
func (h fakeHandle) UsernameWithContext(context.Context) (string, error) {
	return h.user, h.otherErr
}
func (h fakeHandle) CmdlineWithContext(context.Context) (string, error) {
	return h.cmdline, h.otherErr
}
func (h fakeHandle) ExeWithContext(context.Context) (string, error) { return h.exe, h.otherErr }
func (h fakeHandle) CreateTimeWithContext(context.Context) (int64, error) {
	return h.createMS, h.otherErr
}

func providerWithHandle(h processHandle, err error) *systemProvider {
	return &systemProvider{newProcess: func(context.Context, int32) (processHandle, error) { return h, err }}
}

func TestSystemProcess(t *testing.T) {
	t.Parallel()
	created := time.UnixMilli(1_700_000_000_000)

	t.Run("full", func(t *testing.T) {
		t.Parallel()
		p := providerWithHandle(fakeHandle{name: "node", user: "alice", cmdline: "node app.js", exe: "/usr/bin/node", createMS: created.UnixMilli()}, nil)
		got, err := p.Process(t.Context(), 42)
		if err != nil {
			t.Fatal(err)
		}
		want := &ProcessInfo{PID: 42, Name: "node", User: "alice", Cmdline: "node app.js", Exe: "/usr/bin/node", CreateTime: created}
		if d := diff(want, got); d != "" {
			t.Errorf("(-want +got):\n%s", d)
		}
	})

	t.Run("best-effort fields fail", func(t *testing.T) {
		t.Parallel()
		p := providerWithHandle(fakeHandle{name: "sshd", otherErr: fs.ErrPermission}, nil)
		got, err := p.Process(t.Context(), 7)
		if err != nil {
			t.Fatal(err)
		}
		if d := diff(&ProcessInfo{PID: 7, Name: "sshd"}, got); d != "" {
			t.Errorf("(-want +got):\n%s", d)
		}
	})

	t.Run("zero create time ignored", func(t *testing.T) {
		t.Parallel()
		p := providerWithHandle(fakeHandle{name: "x", createMS: 0}, nil)
		got, _ := p.Process(t.Context(), 1)
		if !got.CreateTime.IsZero() {
			t.Errorf("CreateTime = %v, want zero", got.CreateTime)
		}
	})

	t.Run("process gone", func(t *testing.T) {
		t.Parallel()
		_, err := providerWithHandle(nil, process.ErrorProcessNotRunning).Process(t.Context(), 9)
		if !errors.Is(err, ErrProcessNotFound) || !errors.Is(err, process.ErrorProcessNotRunning) {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("name denied", func(t *testing.T) {
		t.Parallel()
		_, err := providerWithHandle(fakeHandle{nameErr: fmt.Errorf("open: %w", os.ErrPermission)}, nil).Process(t.Context(), 9)
		if !errors.Is(err, ErrAccessDenied) {
			t.Errorf("error = %v, want ErrAccessDenied", err)
		}
	})
}

// treeProvider serves handles from a fake process tree.
func treeProvider(tree map[int32]fakeHandle, missing map[int32]error) *systemProvider {
	return &systemProvider{newProcess: func(_ context.Context, pid int32) (processHandle, error) {
		if err, ok := missing[pid]; ok {
			return nil, err
		}
		return tree[pid], nil
	}}
}

func TestSystemAncestors(t *testing.T) {
	t.Parallel()
	tree := map[int32]fakeHandle{
		50: {name: "node", ppid: 40},
		40: {name: "npm", ppid: 30},
		30: {name: "zsh", ppid: 20},
		20: {name: "iTerm2", ppid: 1},
		10: {name: "bash", ppid: 1},
		9:  {name: "child", ppid: 10},
		8:  {name: "loop", ppid: 8},
		7:  {name: "orphan", ppidErr: errors.New("no ppid")},
		6:  {name: "x", ppid: 99},
		5:  {name: "y", ppid: 98},
	}
	missing := map[int32]error{99: process.ErrorProcessNotRunning}
	tree[98] = fakeHandle{nameErr: fs.ErrPermission}
	p := treeProvider(tree, missing)

	tests := []struct {
		pid  int32
		want []ProcessRef
	}{
		{50, []ProcessRef{{40, "npm"}, {30, "zsh"}, {20, "iTerm2"}}}, // capped at MaxAncestors
		{9, []ProcessRef{{10, "bash"}}},                              // stops before PID 1
		{10, nil},                                                    // parent is init
		{8, nil},                                                     // self-reference
		{7, nil},                                                     // ppid unknown
		{6, nil},                                                     // parent exited
		{5, nil},                                                     // parent name denied
	}
	for _, tt := range tests {
		got, err := p.Process(t.Context(), tt.pid)
		if err != nil {
			t.Fatalf("pid %d: %v", tt.pid, err)
		}
		if d := diff(tt.want, got.Ancestors); d != "" {
			t.Errorf("pid %d ancestors (-want +got):\n%s", tt.pid, d)
		}
	}
}

func TestParent(t *testing.T) {
	t.Parallel()
	var nilInfo *ProcessInfo
	if nilInfo.Parent() != nil || (&ProcessInfo{}).Parent() != nil {
		t.Error("Parent of unknown ancestry must be nil")
	}
	p := &ProcessInfo{Ancestors: []ProcessRef{{40, "npm"}, {30, "zsh"}}}
	if got := p.Parent(); got == nil || *got != (ProcessRef{40, "npm"}) {
		t.Errorf("Parent = %v", got)
	}
}

func TestSystemProcessSelf(t *testing.T) {
	t.Parallel()
	pid := int32(os.Getpid())
	got, err := NewSystemProvider().Process(t.Context(), pid)
	if err != nil {
		t.Fatalf("Process(self) error: %v", err)
	}
	if got.PID != pid || got.Name == "" {
		t.Errorf("Process(self) = %+v", got)
	}
}

func TestSystemSockets(t *testing.T) {
	t.Parallel()
	stats := []gnet.ConnectionStat{
		{Type: syscall.SOCK_STREAM, Family: syscall.AF_INET, Laddr: gnet.Addr{IP: "127.0.0.1", Port: 80}, Status: "LISTEN", Pid: 10},
		{Type: syscall.SOCK_DGRAM, Family: syscall.AF_INET6, Laddr: gnet.Addr{IP: "*", Port: 53}, Pid: 11},
		{Type: 99, Laddr: gnet.Addr{IP: "127.0.0.1", Port: 1}},                  // unknown type: dropped
		{Type: syscall.SOCK_STREAM, Laddr: gnet.Addr{IP: "not-an-ip", Port: 1}}, // bad local: dropped
	}
	p := &systemProvider{connections: func(_ context.Context, kind string) ([]gnet.ConnectionStat, error) {
		if kind != "inet" {
			t.Errorf("kind = %q, want inet", kind)
		}
		return stats, nil
	}}
	got, err := p.Sockets(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []RawSocket{
		{Proto: TCP, Local: ap("127.0.0.1:80"), State: StateListen, PID: 10},
		{Proto: UDP, Local: ap("[::]:53"), State: StateNone, PID: 11},
	}
	if d := diff(want, got); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}

	p.connections = func(context.Context, string) ([]gnet.ConnectionStat, error) { return nil, fs.ErrPermission }
	if _, err := p.Sockets(t.Context()); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("error = %v, want ErrAccessDenied", err)
	}
}

func TestFromConnectionStat(t *testing.T) {
	t.Parallel()
	got, ok := fromConnectionStat(gnet.ConnectionStat{
		Type: syscall.SOCK_STREAM, Family: syscall.AF_INET,
		Laddr: gnet.Addr{IP: "10.0.0.1", Port: 5000}, Raddr: gnet.Addr{IP: "10.0.0.2", Port: 6000},
		Status: " established ", Pid: -1,
	})
	want := RawSocket{Proto: TCP, Local: ap("10.0.0.1:5000"), Remote: ap("10.0.0.2:6000"), State: StateEstablished, PID: 0}
	if !ok {
		t.Fatal("not ok")
	}
	if d := diff(want, got); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
}

func TestFromConnectionStatUnconnectedRemote(t *testing.T) {
	t.Parallel()
	// Linux and Windows report listeners with remote 0.0.0.0:0 / [::]:0.
	for _, raddr := range []gnet.Addr{{IP: "0.0.0.0", Port: 0}, {IP: "::", Port: 0}} {
		got, ok := fromConnectionStat(gnet.ConnectionStat{
			Type: syscall.SOCK_STREAM, Family: syscall.AF_INET,
			Laddr: gnet.Addr{IP: "127.0.0.1", Port: 3000}, Raddr: raddr, Status: "LISTEN",
		})
		if !ok || got.Remote.IsValid() {
			t.Errorf("remote %+v: got Remote %v, want unset", raddr, got.Remote)
		}
	}
}

func TestToAddrPort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		a      gnet.Addr
		family uint32
		want   netip.AddrPort
		ok     bool
	}{
		{"empty", gnet.Addr{}, syscall.AF_INET, netip.AddrPort{}, false},
		{"port too large", gnet.Addr{IP: "1.2.3.4", Port: 70000}, syscall.AF_INET, netip.AddrPort{}, false},
		{"ipv4", gnet.Addr{IP: "1.2.3.4", Port: 80}, syscall.AF_INET, ap("1.2.3.4:80"), true},
		{"ipv6 bracketed", gnet.Addr{IP: "[::1]", Port: 80}, syscall.AF_INET6, ap("[::1]:80"), true},
		{"wildcard v4", gnet.Addr{IP: "*", Port: 80}, syscall.AF_INET, ap("0.0.0.0:80"), true},
		{"wildcard v6", gnet.Addr{IP: "*", Port: 80}, syscall.AF_INET6, ap("[::]:80"), true},
		{"empty ip with port", gnet.Addr{IP: "", Port: 80}, syscall.AF_INET, ap("0.0.0.0:80"), true},
		{"v4-mapped is unmapped", gnet.Addr{IP: "::ffff:127.0.0.1", Port: 80}, syscall.AF_INET6, ap("127.0.0.1:80"), true},
		{"invalid ip", gnet.Addr{IP: "localhost", Port: 80}, syscall.AF_INET, netip.AddrPort{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := toAddrPort(tt.a, tt.family)
			if ok != tt.ok || got != tt.want {
				t.Errorf("toAddrPort = %v, %v; want %v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()
	other := errors.New("other")
	if got := classify(other); got != other { //nolint:errorlint // identity is the point of this assertion
		t.Errorf("unknown errors must pass through unchanged, got %v", got)
	}
	if got := classify(process.ErrorProcessNotRunning); !errors.Is(got, ErrProcessNotFound) {
		t.Errorf("got %v", got)
	}
	if got := classify(fs.ErrPermission); !errors.Is(got, ErrAccessDenied) || !errors.Is(got, fs.ErrPermission) {
		t.Errorf("got %v", got)
	}
}

func TestNormalizeState(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]State{"": StateNone, " ": StateNone, "listen": StateListen, "TIME_WAIT": StateTimeWait, "NONE": StateNone, "weird": "WEIRD"} {
		if got := normalizeState(in); got != want {
			t.Errorf("normalizeState(%q) = %q, want %q", in, got, want)
		}
	}
}
