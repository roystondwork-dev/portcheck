//go:build integration

package freeport

import (
	"net"
	"os"
	"testing"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

func TestIntegrationCheckBusyAndFree(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(l.Addr().(*net.TCPAddr).Port)

	f := New()
	st, err := f.Check(t.Context(), port, inspect.TCP)
	if err != nil {
		t.Fatal(err)
	}
	if st.Availability != InUse {
		t.Fatalf("port %d with a listener reported %v", port, st.Availability)
	}
	if len(st.Holders) == 0 || st.Holders[0].PID != int32(os.Getpid()) {
		t.Errorf("holders = %+v, want this process", st.Holders)
	}

	l.Close()
	st, err = f.Check(t.Context(), port, inspect.TCP)
	if err != nil {
		t.Fatal(err)
	}
	if st.Availability != Free {
		t.Errorf("port %d after close reported %v", port, st.Availability)
	}
}

func TestIntegrationFindAndRandom(t *testing.T) {
	f := New()
	spec := portspec.MustParse("40000-40100")
	ports, err := f.Find(t.Context(), spec, inspect.TCP, 3)
	if err != nil || len(ports) != 3 {
		t.Fatalf("Find = %v, %v", ports, err)
	}
	p, err := f.Random(t.Context(), spec, inspect.UDP)
	if err != nil || !spec.Contains(p) {
		t.Fatalf("Random = %d, %v", p, err)
	}
}
