package netinfo

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var errBoom = errors.New("boom")

func TestSystemInterfaces(t *testing.T) {
	t.Parallel()
	up := net.Interface{Index: 1, Name: "en0", Flags: net.FlagUp}
	gone := net.Interface{Index: 2, Name: "en1", Flags: net.FlagUp}
	down := net.Interface{Index: 3, Name: "en2"}
	s := &System{
		interfaces: func() ([]net.Interface, error) { return []net.Interface{up, gone, down}, nil },
		addrs: func(i net.Interface) ([]net.Addr, error) {
			if i.Name == "en1" {
				return nil, errBoom
			}
			return []net.Addr{&net.IPNet{IP: net.ParseIP("192.0.2.10"), Mask: net.CIDRMask(24, 32)}}, nil
		},
	}
	got, err := s.Interfaces()
	if err != nil || len(got) != 1 || got[0].Name != "en0" || len(got[0].Prefixes) != 1 {
		t.Fatalf("Interfaces() = %+v, %v", got, err)
	}
	s.interfaces = func() ([]net.Interface, error) { return nil, errBoom }
	if _, err := s.Interfaces(); !errors.Is(err, errBoom) {
		t.Errorf("error = %v", err)
	}
}

type fakeConn struct {
	net.Conn
	local net.Addr
}

func (c fakeConn) LocalAddr() net.Addr { return c.local }
func (c fakeConn) Close() error        { return nil }

type strAddr string

func (a strAddr) Network() string { return "udp" }
func (a strAddr) String() string  { return string(a) }

func TestSystemPrimary(t *testing.T) {
	t.Parallel()
	s := &System{dial: func(network, _ string) (net.Conn, error) {
		if network == "udp6" {
			return nil, errBoom
		}
		return fakeConn{local: &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 50000}}, nil
	}}
	if a, err := s.Primary("udp4"); err != nil || a != netip.MustParseAddr("192.0.2.10") {
		t.Errorf("udp4 = %v, %v", a, err)
	}
	if _, err := s.Primary("udp6"); err == nil || !strings.Contains(err.Error(), "no 6 route") {
		t.Errorf("udp6 error = %v", err)
	}
	if _, err := s.Primary("tcp"); err == nil {
		t.Error("tcp accepted")
	}
	s.dial = func(string, string) (net.Conn, error) { return fakeConn{local: strAddr("pipe")}, nil }
	if _, err := s.Primary("udp4"); err == nil {
		t.Error("unparsable local address accepted")
	}
	// The real thing: dialing UDP sends nothing, and without a route it fails.
	real := NewSystem()
	if _, err := real.Interfaces(); err != nil {
		t.Errorf("real Interfaces: %v", err)
	}
	if a, err := real.Primary("udp4"); err == nil && !a.Is4() {
		t.Errorf("real udp4 primary = %v", a)
	}
}

func TestGather(t *testing.T) {
	t.Parallel()
	files := map[string]string{"/a": "nameserver 192.0.2.1\n", "/b": "nameserver 192.0.2.2\n"}
	s := &System{
		readFile: func(name string) ([]byte, error) {
			if v, ok := files[name]; ok {
				return []byte(v), nil
			}
			return nil, errBoom
		},
		run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "ok" {
				return []byte("nameserver 192.0.2.3\n"), nil
			}
			return nil, errors.New("no such command")
		},
		dnsSources: []source[netip.Addr]{
			{file: "/a", parse: ParseResolvConf}, {file: "/missing", parse: ParseResolvConf},
			{cmd: []string{"ok"}, parse: ParseResolvConf}, {file: "/b", parse: ParseResolvConf},
			{fn: func(context.Context) ([]netip.Addr, error) { return addrs("192.0.2.4"), nil }},
			{fn: func(context.Context) ([]netip.Addr, error) { return nil, errBoom }},
		},
	}
	got, err := s.DNSServers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(addrs("192.0.2.1", "192.0.2.3", "192.0.2.2", "192.0.2.4"), got, cmpAddr); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
	s.gatewaySources = []source[netip.Addr]{{file: "/missing", parse: ParseProcRoute}, {cmd: []string{"nope"}, parse: ParseRouteGet}}
	if _, err := s.Gateways(t.Context()); !errors.Is(err, errBoom) {
		t.Errorf("all failed: %v, want the first error", err)
	}
	if _, err := s.Neighbors(t.Context()); !errors.Is(err, ErrUnsupported) {
		t.Errorf("no sources: %v", err)
	}
	// The real sources of this OS read without crashing (they may fail in a sandbox).
	real := NewSystem()
	_, _ = real.Gateways(t.Context())
	_, _ = real.DNSServers(t.Context())
	_, _ = real.Neighbors(t.Context())
}

func TestRunCommand(t *testing.T) {
	t.Parallel()
	out, err := runCommand(t.Context(), "go", "version")
	if err != nil || !strings.HasPrefix(string(out), "go version") {
		t.Errorf("go version = %q, %v", out, err)
	}
	if _, err := runCommand(t.Context(), "portcheck-no-such-command"); err == nil {
		t.Error("missing command succeeded")
	}
}

func TestHTTPGet(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("203.0.113.7\n"))
		case "/short":
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("203.0"))
		default:
			http.Error(w, "nope", http.StatusTeapot)
		}
	}))
	defer srv.Close()
	if body, err := HTTPGet(t.Context(), srv.URL+"/ok"); err != nil || body != "203.0.113.7\n" {
		t.Errorf("ok = %q, %v", body, err)
	}
	if _, err := HTTPGet(t.Context(), srv.URL+"/teapot"); err == nil || !strings.Contains(err.Error(), "418") {
		t.Errorf("status error = %v", err)
	}
	if _, err := HTTPGet(t.Context(), srv.URL+"/short"); err == nil {
		t.Error("truncated body accepted")
	}
	if _, err := HTTPGet(t.Context(), "http://\x7f"); err == nil {
		t.Error("bad URL accepted")
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	if _, err := HTTPGet(t.Context(), closed.URL); err == nil {
		t.Error("closed server answered")
	}
}

func TestPublicIPs(t *testing.T) {
	t.Parallel()
	get := func(answers map[string]string) Getter {
		return func(_ context.Context, url string) (string, error) {
			if v, ok := answers[url]; ok {
				return v, nil
			}
			return "", errBoom
		}
	}
	both, err := PublicIPs(t.Context(), get(map[string]string{PublicIPEndpoints[0]: "203.0.113.7\n", PublicIPEndpoints[1]: "2001:db8::7"}))
	if err != nil || len(both) != 2 {
		t.Errorf("both = %v, %v", both, err)
	}
	only4, err := PublicIPs(t.Context(), get(map[string]string{PublicIPEndpoints[0]: "203.0.113.7"}))
	if err != nil || len(only4) != 1 {
		t.Errorf("IPv4 only = %v, %v", only4, err)
	}
	if _, err := PublicIPs(t.Context(), get(nil)); !errors.Is(err, errBoom) {
		t.Errorf("none = %v", err)
	}
	if _, err := PublicIPs(t.Context(), get(map[string]string{PublicIPEndpoints[0]: "<html>", PublicIPEndpoints[1]: "nope"})); err == nil || !strings.Contains(err.Error(), "<html>") {
		t.Errorf("garbage = %v", err)
	}
}
