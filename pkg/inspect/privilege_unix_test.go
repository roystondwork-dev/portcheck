//go:build !windows

package inspect

import "testing"

func TestIsElevatedUnix(t *testing.T) { //nolint:paralleltest // mutates package-level geteuid
	orig := geteuid
	t.Cleanup(func() { geteuid = orig })

	geteuid = func() int { return 0 }
	if !isElevated() {
		t.Error("euid 0 should be elevated")
	}
	geteuid = func() int { return 501 }
	if isElevated() {
		t.Error("euid 501 should not be elevated")
	}
}
