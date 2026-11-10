//go:build integration

package netinfo

import "testing"

// TestSystemIntegration reads the real machine: a loopback interface exists,
// and the primary address, when there is one, belongs to an interface.
func TestSystemIntegration(t *testing.T) {
	t.Parallel()
	src := NewSystem()
	id, err := Collect(t.Context(), src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	loopback := false
	for _, i := range id.Interfaces {
		loopback = loopback || i.Kind == KindLoopback
	}
	if !loopback {
		t.Errorf("no loopback interface in %+v", id.Interfaces)
	}
	if id.Primary4.IsValid() && interfaceOf(id.Interfaces, id.Primary4) == "" {
		t.Errorf("primary %v isn't on any interface", id.Primary4)
	}
	devs, _, err := Devices(t.Context(), src, DeviceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range devs {
		if !d.IP.IsValid() || d.MAC == "" && !d.Self {
			t.Errorf("odd device %+v", d)
		}
	}
}
