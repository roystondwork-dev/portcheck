package containers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

// sampleResponse mirrors a real Docker Engine 29 response for
// `docker run -p 18080:80 -p 127.0.0.1:18443:80 nginx:alpine` plus edge cases.
const sampleResponse = `[
 {"Id":"b1308ab03f9f6f5a2a7c1e0d9b8a7f6e5d4c3b2a1","Names":["/web"],"Image":"nginx:alpine","Ports":[
   {"IP":"127.0.0.1","PrivatePort":80,"PublicPort":18443,"Type":"tcp"},
   {"IP":"0.0.0.0","PrivatePort":80,"PublicPort":18080,"Type":"tcp"},
   {"IP":"::","PrivatePort":80,"PublicPort":18080,"Type":"tcp"},
   {"PrivatePort":9000,"Type":"tcp"}]},
 {"Id":"short","Names":[],"Image":"coredns","Ports":[
   {"IP":"0.0.0.0","PrivatePort":53,"PublicPort":1053,"Type":"udp"},
   {"IP":"0.0.0.0","PrivatePort":99,"PublicPort":1099,"Type":"sctp"},
   {"IP":"","PrivatePort":7,"PublicPort":1007,"Type":"tcp"}]}
]`

// serve starts handler on a TCP server and, except on Windows (where unix
// socket URLs with drive letters are not meaningful), a unix socket. It returns
// the endpoints, primary (unix where available) first.
func serve(t *testing.T, handler http.HandlerFunc) []string {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	tcpEP := "tcp://" + strings.TrimPrefix(ts.URL, "http://")
	if runtime.GOOS == "windows" {
		return []string{tcpEP}
	}
	// macOS limits unix socket paths to ~104 bytes, so keep the directory short.
	dir, err := os.MkdirTemp("", "pcd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return []string{"unix://" + sock, tcpEP}
}

func okHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/containers/json" {
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(sampleResponse))
	}
}

func addr(s string) netip.Addr { a, _ := netip.ParseAddr(s); return a }

var wantPublished = []Published{
	{Port: Port{Proto: inspect.TCP, HostIP: addr("127.0.0.1"), HostPort: 18443, ContainerPort: 80}, Container: web},
	{Port: Port{Proto: inspect.TCP, HostIP: addr("0.0.0.0"), HostPort: 18080, ContainerPort: 80}, Container: web},
	{Port: Port{Proto: inspect.TCP, HostIP: addr("::"), HostPort: 18080, ContainerPort: 80}, Container: web},
	{Port: Port{Proto: inspect.UDP, HostIP: addr("0.0.0.0"), HostPort: 1053, ContainerPort: 53}, Container: dnsCtr},
	{Port: Port{Proto: inspect.TCP, HostPort: 1007, ContainerPort: 7}, Container: dnsCtr},
}

var (
	web    = inspect.Container{ID: "b1308ab03f9f", Name: "web", Image: "nginx:alpine", Runtime: "docker"}
	dnsCtr = inspect.Container{ID: "short", Image: "coredns", Runtime: "docker"}
)

var cmpAddr = cmp.Comparer(func(a, b netip.Addr) bool { return a == b })

func TestPublished(t *testing.T) {
	t.Parallel()
	for _, ep := range serve(t, okHandler(t)) {
		c, err := New(ep)
		if err != nil {
			t.Fatal(err)
		}
		if c.Endpoint() != ep {
			t.Errorf("Endpoint() = %q", c.Endpoint())
		}
		got, err := c.Published(t.Context())
		if err != nil {
			t.Fatalf("%s: %v", ep, err)
		}
		if d := cmp.Diff(wantPublished, got, cmpAddr); d != "" {
			t.Errorf("%s (-want +got):\n%s", ep, d)
		}
	}
}

func TestPublishedErrors(t *testing.T) {
	t.Parallel()
	statusEP := serve(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "nope", http.StatusForbidden) })[0]
	badJSONEP := serve(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{not json")) })[0]
	for name, tc := range map[string]struct{ ep, want string }{
		"status":    {statusEP, "403 Forbidden"},
		"bad json":  {badJSONEP, "decode response"},
		"no socket": {"unix:///nonexistent/dir/docker.sock", "container engine at unix:///nonexistent"},
		// Not the default pipe: GitHub's Windows runners run a real Docker engine there.
		"no pipe": {"npipe:////./pipe/portcheck-no-such-engine", "container engine at npipe"},
	} {
		c, err := New(tc.ep)
		if err != nil {
			t.Fatalf("%s: New: %v", name, err)
		}
		if _, err := c.Published(t.Context()); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want it to contain %q", name, err, tc.want)
		}
	}
	// A malformed base URL fails when building the request.
	c := &Client{endpoint: "x", baseURL: "http://%zz", http: http.DefaultClient}
	if _, err := c.Published(t.Context()); err == nil || !strings.Contains(err.Error(), "request") {
		t.Errorf("bad URL error = %v", err)
	}
}

func TestNewErrors(t *testing.T) {
	t.Parallel()
	for _, ep := range []string{"ssh://host", "unix://", "tcp://", "://bad", "unix://%zz"} {
		if _, err := New(ep); !errors.Is(err, ErrUnsupportedEndpoint) {
			t.Errorf("New(%q) error = %v, want ErrUnsupportedEndpoint", ep, err)
		}
	}
}

func TestRuntimeDetection(t *testing.T) {
	t.Parallel()
	for ep, want := range map[string]string{
		"unix:///run/podman/podman.sock":          "podman",
		"npipe:////./pipe/podman-machine-default": "podman",
		"unix:///var/run/docker.sock":             "docker",
		"tcp://127.0.0.1:2375":                    "docker",
	} {
		c, err := New(ep)
		if err != nil || c.runtime != want {
			t.Errorf("New(%q) runtime = %v, %v; want %s", ep, c, err, want)
		}
	}
}

func sock(proto inspect.Proto, local string, state inspect.State) inspect.Socket {
	return inspect.Socket{RawSocket: inspect.RawSocket{Proto: proto, Local: netip.MustParseAddrPort(local), State: state}}
}

func TestEnrich(t *testing.T) {
	t.Parallel()
	c, _ := New(serve(t, okHandler(t))[0])
	socks := []inspect.Socket{
		sock(inspect.TCP, "[::]:18080", inspect.StateListen),         // dual-stack socket, wildcard publish
		sock(inspect.TCP, "127.0.0.1:18443", inspect.StateListen),    // exact IP match
		sock(inspect.TCP, "10.0.0.1:18443", inspect.StateListen),     // different specific IP: no match
		sock(inspect.UDP, "0.0.0.0:1053", inspect.StateNone),         // UDP publish
		sock(inspect.TCP, "0.0.0.0:1053", inspect.StateListen),       // proto mismatch
		sock(inspect.TCP, "192.168.1.5:1007", inspect.StateListen),   // publish without IP = all addresses
		sock(inspect.TCP, "0.0.0.0:18080", inspect.StateEstablished), // not listening
		sock(inspect.TCP, "0.0.0.0:5432", inspect.StateListen),       // not published
	}
	if err := c.Enrich(t.Context(), socks); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(socks))
	for _, s := range socks {
		name := "-"
		if s.Container != nil {
			name = s.Container.Image
		}
		got = append(got, name)
	}
	want := []string{"nginx:alpine", "nginx:alpine", "-", "coredns", "-", "coredns", "-", "-"}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
	// Each socket gets its own copy.
	socks[0].Container.Name = "mutated"
	if socks[1].Container.Name != "web" {
		t.Error("sockets share a Container value")
	}

	broken, _ := New("unix:///nonexistent/docker.sock")
	if err := broken.Enrich(t.Context(), socks); err == nil {
		t.Error("expected an error from an unreachable engine")
	}
}

func TestEnrichIntegratesWithInspector(t *testing.T) {
	t.Parallel()
	c, _ := New(serve(t, okHandler(t))[0])
	fp := &inspect.FakeProvider{SocketList: []inspect.RawSocket{
		{Proto: inspect.TCP, Local: netip.MustParseAddrPort("[::]:18080"), State: inspect.StateListen, PID: 9},
	}}
	res, err := inspect.New(fp, inspect.WithEnricher(c)).List(t.Context(), inspect.Filter{Name: "web"})
	if err != nil || len(res.Sockets) != 1 || res.Sockets[0].Container.Name != "web" {
		t.Fatalf("List = %+v, %v", res, err)
	}
}

func TestShortID(t *testing.T) {
	t.Parallel()
	if shortID("abc") != "abc" || shortID("0123456789abcdef") != "0123456789ab" {
		t.Error("shortID")
	}
}

func TestEnrichRespectsContext(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	c, _ := New(serve(t, func(http.ResponseWriter, *http.Request) { <-block })[0])
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.Enrich(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}
