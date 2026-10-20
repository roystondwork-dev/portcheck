package freeport

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// fakeLister returns the given sockets filtered like the real Inspector would.
type fakeLister struct {
	sockets []inspect.RawSocket
	err     error
	filters []inspect.Filter
}

func (l *fakeLister) List(ctx context.Context, f inspect.Filter) (*inspect.Result, error) {
	l.filters = append(l.filters, f)
	if l.err != nil {
		return nil, l.err
	}
	return inspect.New(&inspect.FakeProvider{SocketList: l.sockets}).List(ctx, f)
}

func listener(port uint16, pid int32) inspect.RawSocket {
	return inspect.RawSocket{Proto: inspect.TCP, Local: netip.AddrPortFrom(netip.IPv4Unspecified(), port), State: inspect.StateListen, PID: pid}
}

// fakeBinder returns errors per "host:port" and records calls.
type fakeBinder struct {
	mu    sync.Mutex
	errs  map[string]error
	calls []string
}

func (b *fakeBinder) bind(_ context.Context, _ inspect.Proto, addr string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, addr)
	return b.errs[addr]
}

func newTestFinder(l Lister, b *fakeBinder, hosts ...string) *Finder {
	if len(hosts) == 0 {
		hosts = []string{"127.0.0.1"}
	}
	f := New(WithLister(l), WithHosts(hosts...))
	f.bind = b.bind
	return f
}

func addr(host string, port int) string { return net.JoinHostPort(host, strconv.Itoa(port)) }

func TestCheck(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		sockets []inspect.RawSocket
		errs    map[string]error
		hosts   []string
		want    Availability
		holders int
	}{
		{name: "free", want: Free},
		{name: "visible holder", sockets: []inspect.RawSocket{listener(3000, 42)}, want: InUse, holders: 1},
		{name: "bind in use (owner hidden)", errs: map[string]error{addr("127.0.0.1", 3000): errAddrInUse}, want: InUse},
		{name: "bind denied", errs: map[string]error{addr("127.0.0.1", 3000): fs.ErrPermission}, want: Denied},
		{name: "unknown bind error is not free", errs: map[string]error{addr("127.0.0.1", 3000): errors.New("weird")}, want: InUse},
		{
			name:  "unusable address skipped",
			hosts: []string{"::1", "127.0.0.1"},
			errs:  map[string]error{addr("::1", 3000): errAddrNotAvail},
			want:  Free,
		},
		{
			name:  "second host in use",
			hosts: []string{"127.0.0.1", "0.0.0.0"},
			errs:  map[string]error{addr("0.0.0.0", 3000): errAddrInUse},
			want:  InUse,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFinder(&fakeLister{sockets: tt.sockets}, &fakeBinder{errs: tt.errs}, tt.hosts...)
			st, err := f.Check(t.Context(), 3000, inspect.TCP)
			if err != nil {
				t.Fatal(err)
			}
			if st.Availability != tt.want || len(st.Holders) != tt.holders || st.Port != 3000 || st.Proto != inspect.TCP {
				t.Errorf("Check = %+v, want availability %v with %d holders", st, tt.want, tt.holders)
			}
			if st.Free() != (tt.want == Free) {
				t.Error("Free() inconsistent with Availability")
			}
		})
	}
}

func TestCheckSkipsBindWhenHolderVisible(t *testing.T) {
	t.Parallel()
	b := &fakeBinder{}
	f := newTestFinder(&fakeLister{sockets: []inspect.RawSocket{listener(80, 1)}}, b)
	if _, err := f.Check(t.Context(), 80, inspect.TCP); err != nil {
		t.Fatal(err)
	}
	if len(b.calls) != 0 {
		t.Errorf("bind called %v, want no calls", b.calls)
	}
}

func TestCheckQueriesListeningSocketsOfProto(t *testing.T) {
	t.Parallel()
	l := &fakeLister{}
	f := newTestFinder(l, &fakeBinder{})
	if _, err := f.Check(t.Context(), 53, inspect.UDP); err != nil {
		t.Fatal(err)
	}
	got := l.filters[0]
	if !got.ListeningOnly || !cmp.Equal(got.Protos, []inspect.Proto{inspect.UDP}) || got.Ports.String() != "53" {
		t.Errorf("filter = %+v", got)
	}
}

func TestCheckAll(t *testing.T) {
	t.Parallel()
	l := &fakeLister{sockets: []inspect.RawSocket{listener(81, 7)}}
	b := &fakeBinder{errs: map[string]error{addr("127.0.0.1", 83): fs.ErrPermission}}
	f := newTestFinder(l, b)
	sts, err := f.CheckAll(t.Context(), portspec.MustParse("80-83"), inspect.TCP)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(sts))
	for i, st := range sts {
		got[i] = fmt.Sprintf("%d:%s:%d", st.Port, st.Availability, len(st.Holders))
	}
	if diff := cmp.Diff([]string{"80:free:0", "81:in-use:1", "82:free:0", "83:denied:0"}, got); diff != "" {
		t.Errorf("(-want +got):\n%s", diff)
	}
	if len(l.filters) != 1 {
		t.Errorf("lister called %d times, want 1", len(l.filters))
	}
}

func TestCheckAllCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	f := newTestFinder(nil, &fakeBinder{})
	f.bind = func(context.Context, inspect.Proto, string) error { cancel(); return nil }
	if _, err := f.CheckAll(ctx, portspec.MustParse("1-5"), inspect.TCP); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	failing := newTestFinder(&fakeLister{err: boom}, &fakeBinder{})
	spec := portspec.MustParse("1-10")

	if _, err := failing.Check(t.Context(), 1, inspect.TCP); !errors.Is(err, boom) {
		t.Errorf("Check error = %v", err)
	}
	if _, err := failing.CheckAll(t.Context(), spec, inspect.TCP); !errors.Is(err, boom) {
		t.Errorf("CheckAll error = %v", err)
	}
	if _, err := failing.Find(t.Context(), spec, inspect.TCP, 1); !errors.Is(err, boom) {
		t.Errorf("Find error = %v", err)
	}
	if _, err := failing.Random(t.Context(), spec, inspect.TCP); !errors.Is(err, boom) {
		t.Errorf("Random error = %v", err)
	}

	f := newTestFinder(nil, &fakeBinder{})
	if _, err := f.Check(t.Context(), 1, "sctp"); !errors.Is(err, ErrUnsupportedProto) {
		t.Errorf("Check proto error = %v", err)
	}
	if _, err := f.CheckAll(t.Context(), spec, "sctp"); !errors.Is(err, ErrUnsupportedProto) {
		t.Errorf("CheckAll proto error = %v", err)
	}
	if _, err := f.Find(t.Context(), spec, "sctp", 1); !errors.Is(err, ErrUnsupportedProto) {
		t.Errorf("Find proto error = %v", err)
	}
	if _, err := f.Random(t.Context(), spec, "sctp"); !errors.Is(err, ErrUnsupportedProto) {
		t.Errorf("Random proto error = %v", err)
	}
}

func TestFind(t *testing.T) {
	t.Parallel()
	l := &fakeLister{sockets: []inspect.RawSocket{listener(3000, 1), listener(3002, 2)}}
	b := &fakeBinder{errs: map[string]error{addr("127.0.0.1", 3001): errAddrInUse}}
	f := newTestFinder(l, b)
	spec := portspec.MustParse("3000-3010")

	got, err := f.Find(t.Context(), spec, inspect.TCP, 2)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]uint16{3003, 3004}, got); diff != "" {
		t.Errorf("Find (-want +got):\n%s", diff)
	}

	got, err = f.Find(t.Context(), spec, inspect.TCP, 0) // n < 1 means 1
	if err != nil || !cmp.Equal(got, []uint16{3003}) {
		t.Errorf("Find n=0 = %v, %v", got, err)
	}

	got, err = f.Find(t.Context(), portspec.MustParse("3000-3003"), inspect.TCP, 5)
	if !errors.Is(err, ErrNoFreePort) || !cmp.Equal(got, []uint16{3003}) {
		t.Errorf("Find short = %v, %v; want [3003], ErrNoFreePort", got, err)
	}
	if err != nil && err.Error() != "found 1 of 5 in 3000-3003: no free port found" {
		t.Errorf("error message = %q", err)
	}
}

func TestFindCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	b := &fakeBinder{errs: map[string]error{}}
	f := newTestFinder(nil, b)
	// Cancel as soon as the first port has been tried.
	f.bind = func(context.Context, inspect.Proto, string) error {
		cancel()
		return errAddrInUse
	}
	_, err := f.Find(ctx, portspec.MustParse("1-100"), inspect.TCP, 1)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestRandom(t *testing.T) {
	t.Parallel()
	l := &fakeLister{sockets: []inspect.RawSocket{listener(5000, 1)}}
	f := newTestFinder(l, &fakeBinder{})
	picks := []int{0, 0, 3} // 5000 (held), 5000 (held), 5003
	f.randN = func(n int) int {
		if n != 10 {
			t.Errorf("randN(%d), want 10", n)
		}
		p := picks[0]
		picks = picks[1:]
		return p
	}
	got, err := f.Random(t.Context(), portspec.MustParse("5000-5009"), inspect.TCP)
	if err != nil || got != 5003 {
		t.Errorf("Random = %d, %v; want 5003", got, err)
	}
}

func TestRandomExhausted(t *testing.T) {
	t.Parallel()
	b := &fakeBinder{errs: map[string]error{addr("127.0.0.1", 7000): errAddrInUse}}
	f := newTestFinder(nil, b)
	f.randN = func(int) int { return 0 }
	if _, err := f.Random(t.Context(), portspec.Single(7000), inspect.TCP); !errors.Is(err, ErrNoFreePort) {
		t.Errorf("error = %v", err)
	}
	if len(b.calls) != 1 {
		t.Errorf("attempts = %d, want min(spec size, max) = 1", len(b.calls))
	}

	b2 := &fakeBinder{errs: map[string]error{}}
	f2 := newTestFinder(nil, b2)
	f2.bind = func(context.Context, inspect.Proto, string) error {
		b2.calls = append(b2.calls, "")
		return errAddrInUse
	}
	f2.randN = func(int) int { return 0 }
	if _, err := f2.Random(t.Context(), portspec.MustParse("all"), inspect.TCP); !errors.Is(err, ErrNoFreePort) {
		t.Errorf("error = %v", err)
	}
	if len(b2.calls) != maxRandomAttempts {
		t.Errorf("attempts = %d, want %d", len(b2.calls), maxRandomAttempts)
	}

	if _, err := f.Random(t.Context(), portspec.Spec{}, inspect.TCP); !errors.Is(err, ErrNoFreePort) {
		t.Errorf("empty spec error = %v", err)
	}
}

func TestRandomCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f := newTestFinder(nil, &fakeBinder{})
	if _, err := f.Random(ctx, portspec.MustParse("1-10"), inspect.TCP); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v", err)
	}
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()
	f := New()
	if f.lister == nil || f.bind == nil || f.randN == nil {
		t.Error("New left defaults unset")
	}
	if !cmp.Equal(f.hosts, DefaultHosts) {
		t.Errorf("hosts = %v", f.hosts)
	}
	if r := f.randN(5); r < 0 || r >= 5 {
		t.Errorf("randN(5) = %d", r)
	}
	if New(WithLister(nil)).lister != nil {
		t.Error("WithLister(nil) should disable the lister")
	}
}

func TestAvailabilityString(t *testing.T) {
	t.Parallel()
	for a, want := range map[Availability]string{Free: "free", InUse: "in-use", Denied: "denied", Availability(9): "unknown(9)"} {
		if a.String() != want {
			t.Errorf("%d.String() = %q, want %q", int(a), a.String(), want)
		}
	}
}

func TestClassifyBindErr(t *testing.T) {
	t.Parallel()
	wrap := func(err error) error { return &net.OpError{Op: "listen", Err: fmt.Errorf("bind: %w", err)} }
	tests := []struct {
		err  error
		want bindResult
	}{
		{nil, bindOK},
		{wrap(errAddrInUse), bindInUse},
		{wrap(errAddrNotAvail), bindSkip},
		{wrap(errAFNoSupport), bindSkip},
		{wrap(errAccess), bindDenied},
		{wrap(fs.ErrPermission), bindDenied},
		{errors.New("other"), bindInUse},
	}
	for _, tt := range tests {
		if got := classifyBindErr(tt.err); got != tt.want {
			t.Errorf("classifyBindErr(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestBindOS(t *testing.T) {
	t.Parallel()
	for _, proto := range []inspect.Proto{inspect.TCP, inspect.UDP} {
		if err := bindOS(t.Context(), proto, "127.0.0.1:0"); err != nil {
			t.Errorf("bindOS(%s, ephemeral) = %v", proto, err)
		}
	}

	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	if err := bindOS(t.Context(), inspect.TCP, tcp.Addr().String()); classifyBindErr(err) != bindInUse {
		t.Errorf("bindOS on busy TCP port = %v, want address in use", err)
	}

	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	if err := bindOS(t.Context(), inspect.UDP, udp.LocalAddr().String()); classifyBindErr(err) != bindInUse {
		t.Errorf("bindOS on busy UDP port = %v, want address in use", err)
	}
}
