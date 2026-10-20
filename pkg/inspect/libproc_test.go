package inspect

import (
	"context"
	"encoding/binary"
	"errors"
	"net/netip"
	"slices"
	"testing"

	gnet "github.com/shirou/gopsutil/v4/net"
)

// sockBuf builds a struct socket_fdinfo buffer using the documented offsets.
type sockBuf struct {
	family, protocol, kind uint32
	vflag                  byte
	lport, fport           uint16
	laddr, faddr           [16]byte
	state                  uint32
}

func (s sockBuf) bytes() []byte {
	b := make([]byte, socketFDInfoSize)
	le := binary.LittleEndian
	psi := b[psiOffset:]
	le.PutUint32(psi[soiFamilyOffset:], s.family)
	le.PutUint32(psi[soiProtocolOffset:], s.protocol)
	le.PutUint32(psi[soiKindOffset:], s.kind)
	in := psi[soiProtoOffset:]
	in[insiVFlagOffset] = s.vflag
	// ports: network byte order in the low 16 bits of an int
	in[insiLPortOffset], in[insiLPortOffset+1] = byte(s.lport>>8), byte(s.lport)
	in[insiFPortOffset], in[insiFPortOffset+1] = byte(s.fport>>8), byte(s.fport)
	copy(in[insiLAddrOffset:], s.laddr[:])
	copy(in[insiFAddrOffset:], s.faddr[:])
	le.PutUint32(in[tcpsiStateOffset:], s.state)
	return b
}

func v4(a string) (b [16]byte) {
	ip := netip.MustParseAddr(a).As4()
	copy(b[12:], ip[:])
	return b
}

func v6(a string) [16]byte { return netip.MustParseAddr(a).As16() }

func TestParseSocketFDInfo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   sockBuf
		want RawSocket
		ok   bool
	}{
		{"tcp4 listen", sockBuf{family: afInet, protocol: ipprotoTCP, kind: sockinfoTCP, vflag: iniIPv4, lport: 3000, laddr: v4("127.0.0.1"), state: 1},
			RawSocket{Proto: TCP, Local: ap("127.0.0.1:3000"), State: StateListen}, true},
		{"tcp6 established", sockBuf{family: afInet6, protocol: ipprotoTCP, kind: sockinfoTCP, vflag: 2, lport: 5000, fport: 6000, laddr: v6("::1"), faddr: v6("::1"), state: 4},
			RawSocket{Proto: TCP, Local: ap("[::1]:5000"), Remote: ap("[::1]:6000"), State: StateEstablished}, true},
		{"tcp6 socket with v4 flow", sockBuf{family: afInet6, protocol: ipprotoTCP, kind: sockinfoTCP, vflag: iniIPv4, lport: 80, fport: 51000, laddr: v4("10.0.0.1"), faddr: v4("10.0.0.2"), state: 10},
			RawSocket{Proto: TCP, Local: ap("10.0.0.1:80"), Remote: ap("10.0.0.2:51000"), State: StateTimeWait}, true},
		{"v4-mapped v6 address", sockBuf{family: afInet6, protocol: ipprotoTCP, kind: sockinfoTCP, vflag: 2, lport: 22, laddr: v6("::ffff:192.0.2.1"), state: 1},
			RawSocket{Proto: TCP, Local: ap("192.0.2.1:22"), State: StateListen}, true},
		{"tcp6 wildcard", sockBuf{family: afInet6, protocol: ipprotoTCP, kind: sockinfoTCP, vflag: 3, lport: 8080, state: 1},
			RawSocket{Proto: TCP, Local: ap("[::]:8080"), State: StateListen}, true},
		{"udp4 unconnected", sockBuf{family: afInet, protocol: ipprotoUDP, kind: sockinfoIn, vflag: iniIPv4, lport: 5353},
			RawSocket{Proto: UDP, Local: ap("0.0.0.0:5353"), State: StateNone}, true},
		{"unknown tcp state", sockBuf{family: afInet, protocol: ipprotoTCP, kind: sockinfoTCP, vflag: iniIPv4, lport: 1, state: 42},
			RawSocket{Proto: TCP, Local: ap("0.0.0.0:1"), State: "STATE_42"}, true},
		{"udp not bound to a port", sockBuf{family: afInet, protocol: ipprotoUDP, kind: sockinfoIn}, RawSocket{}, false},
		{"unix socket", sockBuf{family: 1, protocol: 0, kind: 3, lport: 1}, RawSocket{}, false},
		{"tcp with wrong kind", sockBuf{family: afInet, protocol: ipprotoTCP, kind: sockinfoIn, lport: 1}, RawSocket{}, false},
		{"icmp", sockBuf{family: afInet, protocol: 1, kind: sockinfoIn, lport: 1}, RawSocket{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseSocketFDInfo(tt.in.bytes())
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if d := diff(tt.want, got); d != "" {
				t.Errorf("(-want +got):\n%s", d)
			}
		})
	}
	if _, ok := parseSocketFDInfo(make([]byte, socketFDInfoSize-1)); ok {
		t.Error("short buffer accepted")
	}
}

func TestParseProcFDs(t *testing.T) {
	t.Parallel()
	b := make([]byte, 2*procFDInfoSize+3) // trailing partial entry is ignored
	binary.LittleEndian.PutUint32(b[0:], 3)
	binary.LittleEndian.PutUint32(b[4:], proxFDTypeSocket)
	binary.LittleEndian.PutUint32(b[8:], 7)
	binary.LittleEndian.PutUint32(b[12:], 1)
	if got, want := parseProcFDs(b), []procFD{{3, proxFDTypeSocket}, {7, 1}}; !slices.Equal(got, want) {
		t.Errorf("parseProcFDs = %v, want %v", got, want)
	}
}

// fakeLibproc serves canned data.
type fakeLibproc struct {
	pids    []int32
	pidsErr error
	fds     map[int32][]procFD
	socks   map[[2]int32][]byte
	onFDs   func()
}

func (f fakeLibproc) listPIDs() ([]int32, error) { return f.pids, f.pidsErr }

func (f fakeLibproc) listFDs(pid int32) ([]procFD, error) {
	if f.onFDs != nil {
		f.onFDs()
	}
	fds, ok := f.fds[pid]
	if !ok {
		return nil, errors.New("EPERM")
	}
	return fds, nil
}

func (f fakeLibproc) socketInfo(pid, fd int32) ([]byte, error) {
	b, ok := f.socks[[2]int32{pid, fd}]
	if !ok {
		return nil, errors.New("gone")
	}
	return b, nil
}

func TestLibprocSockets(t *testing.T) {
	t.Parallel()
	listen := sockBuf{family: afInet, protocol: ipprotoTCP, kind: sockinfoTCP, vflag: iniIPv4, lport: 3000, laddr: v4("127.0.0.1"), state: 1}.bytes()
	unix := sockBuf{family: 1}.bytes()
	lp := fakeLibproc{
		pids: []int32{100, 200, 300},
		fds: map[int32][]procFD{
			100: {{1, 1}, {5, proxFDTypeSocket}, {6, proxFDTypeSocket}, {7, proxFDTypeSocket}}, // vnode, tcp, unix, vanished
			300: {{9, proxFDTypeSocket}},
			// 200: not readable (another user's process)
		},
		socks: map[[2]int32][]byte{{100, 5}: listen, {100, 6}: unix, {300, 9}: listen},
	}
	got, err := libprocSockets(t.Context(), lp)
	if err != nil {
		t.Fatal(err)
	}
	want := []RawSocket{
		{Proto: TCP, Local: ap("127.0.0.1:3000"), State: StateListen, PID: 100},
		{Proto: TCP, Local: ap("127.0.0.1:3000"), State: StateListen, PID: 300},
	}
	if d := diff(want, got); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}

	if _, err := libprocSockets(t.Context(), fakeLibproc{pidsErr: errors.New("boom")}); err == nil {
		t.Error("listPIDs error not returned")
	}

	ctx, cancel := context.WithCancel(t.Context())
	lp.onFDs = cancel
	if _, err := libprocSockets(ctx, lp); !errors.Is(err, context.Canceled) {
		t.Errorf("cancel error = %v", err)
	}
}

func TestSystemSocketsPrefersNative(t *testing.T) {
	t.Parallel()
	nativeSock := RawSocket{Proto: TCP, Local: ap("127.0.0.1:1"), State: StateListen, PID: 1}
	calledFallback := false
	p := &systemProvider{
		native: func(context.Context) ([]RawSocket, error) { return []RawSocket{nativeSock}, nil },
		connections: func(context.Context, string) ([]gnet.ConnectionStat, error) {
			calledFallback = true
			return nil, nil
		},
	}
	got, err := p.Sockets(t.Context())
	if err != nil || len(got) != 1 || calledFallback {
		t.Errorf("native path: %v %v fallback=%v", got, err, calledFallback)
	}

	p.native = func(context.Context) ([]RawSocket, error) { return nil, errors.New("libproc unavailable") }
	if _, err := p.Sockets(t.Context()); err != nil || !calledFallback {
		t.Errorf("fallback not used: err=%v fallback=%v", err, calledFallback)
	}
}
