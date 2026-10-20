//go:build integration && darwin

package inspect

import (
	"fmt"
	"testing"

	gnet "github.com/shirou/gopsutil/v4/net"
)

// TestIntegrationNativeMatchesLsof checks that the libproc listing reports the
// same listening sockets as lsof (via gopsutil) on the live system. Sockets
// can appear or vanish between the two calls, so the comparison is retried.
func TestIntegrationNativeMatchesLsof(t *testing.T) {
	key := func(s RawSocket) string { return fmt.Sprintf("%s %s %d", s.Proto, s.Local, s.PID) }
	listening := func(socks []RawSocket) map[string]bool {
		m := map[string]bool{}
		for _, s := range socks {
			if s.IsListening() {
				m[key(s)] = true
			}
		}
		return m
	}
	lsof := &systemProvider{connections: gnet.ConnectionsWithContext}

	var onlyNative, onlyLsof []string
	for range 3 {
		native, err := nativeSockets(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		viaLsof, err := lsof.Sockets(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		a, b := listening(native), listening(viaLsof)
		onlyNative, onlyLsof = nil, nil
		for k := range a {
			if !b[k] {
				onlyNative = append(onlyNative, k)
			}
		}
		for k := range b {
			if !a[k] {
				onlyLsof = append(onlyLsof, k)
			}
		}
		if len(onlyNative) == 0 && len(onlyLsof) == 0 {
			t.Logf("native and lsof agree on %d listening sockets", len(a))
			return
		}
	}
	t.Errorf("listings differ.\nonly native: %v\nonly lsof: %v", onlyNative, onlyLsof)
}
