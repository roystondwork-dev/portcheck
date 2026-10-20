package inspect

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

func ap(s string) netip.AddrPort { return netip.MustParseAddrPort(s) }

var (
	nodeProc     = &ProcessInfo{PID: 100, Name: "node", User: "alice"}
	postgresProc = &ProcessInfo{PID: 200, Name: "postgres", User: "postgres"}
	dnsProc      = &ProcessInfo{PID: 300, Name: "mDNSResponder", User: "root"}

	sNodeListen     = RawSocket{Proto: TCP, Local: ap("127.0.0.1:3000"), State: StateListen, PID: 100}
	sNodeListen6    = RawSocket{Proto: TCP, Local: ap("[::1]:3000"), State: StateListen, PID: 100}
	sNodeConn       = RawSocket{Proto: TCP, Local: ap("127.0.0.1:3000"), Remote: ap("127.0.0.1:51000"), State: StateEstablished, PID: 100}
	sPostgres       = RawSocket{Proto: TCP, Local: ap("0.0.0.0:5432"), State: StateListen, PID: 200}
	sDNS            = RawSocket{Proto: UDP, Local: ap("0.0.0.0:5353"), State: StateNone, PID: 300}
	sUnknownOwner   = RawSocket{Proto: TCP, Local: ap("0.0.0.0:22"), State: StateListen, PID: 0}
	sClientOutbound = RawSocket{Proto: TCP, Local: ap("10.0.0.5:52000"), Remote: ap("1.1.1.1:443"), State: StateEstablished, PID: 100}
)

func newFake() *FakeProvider {
	return &FakeProvider{
		SocketList: []RawSocket{sPostgres, sNodeConn, sDNS, sNodeListen, sUnknownOwner, sNodeListen6, sClientOutbound},
		Processes:  map[int32]*ProcessInfo{100: nodeProc, 200: postgresProc, 300: dnsProc},
	}
}

func rawOf(socks []Socket) []RawSocket {
	out := make([]RawSocket, len(socks))
	for i, s := range socks {
		out[i] = s.RawSocket
	}
	return out
}

func TestListAllSortedAndEnriched(t *testing.T) {
	t.Parallel()
	res, err := New(newFake()).List(t.Context(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	want := []RawSocket{sUnknownOwner, sNodeListen, sNodeConn, sNodeListen6, sDNS, sPostgres, sClientOutbound}
	if d := diff(want, rawOf(res.Sockets)); d != "" {
		t.Errorf("order (-want +got):\n%s", d)
	}
	if !res.Partial {
		t.Error("Partial = false, want true (a socket has no PID)")
	}
	if res.Sockets[0].Process != nil {
		t.Error("socket without PID should have nil Process")
	}
	if d := diff(nodeProc, res.Sockets[1].Process); d != "" {
		t.Errorf("process (-want +got):\n%s", d)
	}
	if res.Sockets[1].Port() != 3000 {
		t.Errorf("Port() = %d", res.Sockets[1].Port())
	}
}

func TestListFilters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		f    Filter
		want []RawSocket
	}{
		{"ports", Filter{Ports: portspec.MustParse("5000-6000")}, []RawSocket{sDNS, sPostgres}},
		{"protos", Filter{Protos: []Proto{UDP}}, []RawSocket{sDNS}},
		{"states", Filter{States: []State{StateEstablished}}, []RawSocket{sNodeConn, sClientOutbound}},
		{"pids", Filter{PIDs: []int32{200, 300}}, []RawSocket{sDNS, sPostgres}},
		{"listening", Filter{ListeningOnly: true}, []RawSocket{sUnknownOwner, sNodeListen, sNodeListen6, sDNS, sPostgres}},
		{"name glob case-insensitive", Filter{Name: "NO*"}, []RawSocket{sNodeListen, sNodeConn, sNodeListen6, sClientOutbound}},
		{"name exact", Filter{Name: "postgres"}, []RawSocket{sPostgres}},
		{"name no match", Filter{Name: "nginx"}, []RawSocket{}},
		{"combined", Filter{Ports: portspec.Single(3000), ListeningOnly: true, Protos: []Proto{TCP}}, []RawSocket{sNodeListen, sNodeListen6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := New(newFake()).List(t.Context(), tt.f)
			if err != nil {
				t.Fatal(err)
			}
			if d := diff(tt.want, rawOf(res.Sockets)); d != "" {
				t.Errorf("(-want +got):\n%s", d)
			}
		})
	}
}

func TestListNotPartialWhenAllOwnersKnown(t *testing.T) {
	t.Parallel()
	res, err := New(newFake()).List(t.Context(), Filter{Ports: portspec.Single(5432)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Partial {
		t.Error("Partial = true, want false")
	}
}

func TestListDeduplicatesAndCachesProcesses(t *testing.T) {
	t.Parallel()
	fp := &FakeProvider{
		SocketList: []RawSocket{sNodeListen, sNodeListen, sNodeConn, sNodeListen6},
		Processes:  map[int32]*ProcessInfo{100: nodeProc},
	}
	res, err := New(fp, WithConcurrency(1)).List(t.Context(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sockets) != 3 {
		t.Errorf("got %d sockets, want 3 after de-duplication", len(res.Sockets))
	}
	if fp.ProcessCalls() != 1 {
		t.Errorf("Process called %d times, want 1 (one lookup per PID)", fp.ProcessCalls())
	}
}

func TestListProcessErrors(t *testing.T) {
	t.Parallel()
	fp := &FakeProvider{
		SocketList: []RawSocket{sNodeListen, sDNS, sPostgres},
		Processes:  map[int32]*ProcessInfo{300: dnsProc},
		ProcessErrs: map[int32]error{
			100: errors.New("transient failure"),
			200: ErrAccessDenied,
		},
	}
	res, err := New(fp).List(t.Context(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Partial {
		t.Error("Partial = false, want true after access denied")
	}
	for _, s := range res.Sockets {
		if gotNil, wantNil := s.Process == nil, s.PID != 300; gotNil != wantNil {
			t.Errorf("pid %d: Process nil = %v, want %v", s.PID, gotNil, wantNil)
		}
	}
}

func TestListProcessNotFoundIsNotPartial(t *testing.T) {
	t.Parallel()
	fp := &FakeProvider{SocketList: []RawSocket{sPostgres}} // PID 200 has exited
	res, err := New(fp).List(t.Context(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Partial || len(res.Sockets) != 1 || res.Sockets[0].Process != nil {
		t.Errorf("unexpected result %+v", res)
	}
}

func TestListErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")

	_, err := New(&FakeProvider{SocketsErr: boom}).List(t.Context(), Filter{})
	if !errors.Is(err, boom) {
		t.Errorf("Sockets error not propagated: %v", err)
	}

	_, err = New(newFake()).List(t.Context(), Filter{Name: "[unterminated"})
	if !errors.Is(err, ErrBadFilter) {
		t.Errorf("bad filter error = %v, want ErrBadFilter", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = New(newFake()).List(ctx, Filter{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("canceled context error = %v", err)
	}
}

// cancelingProvider cancels the context after listing sockets so that the
// process lookup phase observes cancellation.
type cancelingProvider struct {
	*FakeProvider
	cancel context.CancelFunc
}

func (c cancelingProvider) Sockets(ctx context.Context) ([]RawSocket, error) {
	s, err := c.FakeProvider.Sockets(ctx)
	c.cancel()
	return s, err
}

func TestListCanceledDuringLookup(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	fp := newFake()
	_, err := New(cancelingProvider{fp, cancel}).List(ctx, Filter{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if fp.ProcessCalls() != 0 {
		t.Errorf("Process called %d times after cancellation", fp.ProcessCalls())
	}
}

func TestIsListening(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		s    RawSocket
		want bool
	}{
		{"tcp listen", RawSocket{Proto: TCP, State: StateListen}, true},
		{"tcp established", RawSocket{Proto: TCP, State: StateEstablished}, false},
		{"udp unconnected", RawSocket{Proto: UDP, Local: ap("0.0.0.0:53")}, true},
		{"udp remote port 0", RawSocket{Proto: UDP, Remote: ap("0.0.0.0:0")}, true},
		{"udp connected", RawSocket{Proto: UDP, Remote: ap("8.8.8.8:53")}, false},
		{"unknown proto", RawSocket{Proto: "sctp", State: StateListen}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.s.IsListening(); got != tt.want {
				t.Errorf("IsListening() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterValidate(t *testing.T) {
	t.Parallel()
	if err := (Filter{Name: "node*"}).Validate(); err != nil {
		t.Errorf("valid pattern rejected: %v", err)
	}
	if err := (Filter{}).Validate(); err != nil {
		t.Errorf("empty filter rejected: %v", err)
	}
	if err := (Filter{Name: "["}).Validate(); !errors.Is(err, ErrBadFilter) {
		t.Errorf("invalid pattern error = %v", err)
	}
}

func TestOptions(t *testing.T) {
	t.Parallel()
	if got := New(nil).concurrency; got != 16 {
		t.Errorf("default concurrency = %d", got)
	}
	if got := New(nil, WithConcurrency(4)).concurrency; got != 4 {
		t.Errorf("WithConcurrency(4) = %d", got)
	}
	if got := New(nil, WithConcurrency(0)).concurrency; got != 16 {
		t.Errorf("WithConcurrency(0) should be ignored, got %d", got)
	}
	if in := NewSystem(); in.provider == nil {
		t.Error("NewSystem has nil provider")
	}
}

func TestCompareSocketsTieBreakers(t *testing.T) {
	t.Parallel()
	a := Socket{RawSocket: RawSocket{Proto: TCP, Local: ap("127.0.0.1:80"), State: StateEstablished, PID: 1}}
	b := a
	b.State = StateTimeWait
	if compareSockets(a, b) >= 0 {
		t.Error("state tie-breaker not applied")
	}
	c := a
	c.PID = 2
	if compareSockets(a, c) >= 0 {
		t.Error("pid tie-breaker not applied")
	}
	if compareSockets(a, a) != 0 {
		t.Error("identical sockets should compare equal")
	}
}

// diff compares values, treating netip types as comparable values.
func diff(want, got any) string {
	return cmp.Diff(want, got, cmpopts.EquateComparable(netip.AddrPort{}, netip.Addr{}))
}
