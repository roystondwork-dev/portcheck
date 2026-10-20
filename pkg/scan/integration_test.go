//go:build integration

package scan

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// TestIntegrationLoopback scans real loopback ports: one with a listener and
// one that was just closed.
func TestIntegrationLoopback(t *testing.T) {
	open, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer open.Close()
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := uint16(closed.Addr().(*net.TCPAddr).Port)
	closed.Close()
	openPort := uint16(open.Addr().(*net.TCPAddr).Port)

	targets, err := Expand(t.Context(), net.DefaultResolver, []string{"127.0.0.1", "localhost"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Addr != netip.MustParseAddr("127.0.0.1") {
		t.Fatalf("targets = %v, want localhost deduplicated into 127.0.0.1", targets)
	}

	spec := portspec.New(portspec.Range{Start: openPort, End: openPort}, portspec.Range{Start: closedPort, End: closedPort})
	got := map[uint16]State{}
	for r := range Scan(t.Context(), targets, spec, Options{Timeout: 2 * time.Second}) {
		got[r.Port] = r.State
	}
	if got[openPort] != Open || got[closedPort] != Closed {
		t.Errorf("results = %v; want %d open, %d closed", got, openPort, closedPort)
	}
}
