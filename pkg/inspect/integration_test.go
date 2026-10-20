//go:build integration

package inspect

import (
	"net"
	"os"
	"testing"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// TestIntegrationFindsOwnListeners opens real sockets and checks that the
// system provider attributes them to this test process.
func TestIntegrationFindsOwnListeners(t *testing.T) {
	tcp, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tcp.Close() })
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { udp.Close() })

	tcpPort := uint16(tcp.Addr().(*net.TCPAddr).Port)
	udpPort := uint16(udp.LocalAddr().(*net.UDPAddr).Port)
	self := int32(os.Getpid())

	res, err := NewSystem().List(t.Context(), Filter{
		Ports:         portspec.New(portspec.Range{Start: tcpPort, End: tcpPort}, portspec.Range{Start: udpPort, End: udpPort}),
		PIDs:          []int32{self},
		ListeningOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var gotTCP, gotUDP bool
	for _, s := range res.Sockets {
		if s.Process == nil || s.Process.PID != self || s.Process.Name == "" {
			t.Errorf("socket %+v not attributed to self (pid %d): %+v", s.RawSocket, self, s.Process)
		}
		// `go test` starts this binary, so its parent is known and is not init.
		if parent := s.Process.Parent(); parent == nil || parent.PID != int32(os.Getppid()) || parent.Name == "" {
			t.Errorf("parent = %+v, want pid %d", parent, os.Getppid())
		}
		switch {
		case s.Proto == TCP && s.Port() == tcpPort:
			gotTCP = true
		case s.Proto == UDP && s.Port() == udpPort:
			gotUDP = true
		}
	}
	if !gotTCP {
		t.Errorf("TCP listener on %d not found in %+v", tcpPort, res.Sockets)
	}
	if !gotUDP {
		t.Errorf("UDP socket on %d not found in %+v", udpPort, res.Sockets)
	}
}
