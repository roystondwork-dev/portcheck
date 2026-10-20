//go:build windows

package inspect

import "testing"

func TestIsElevatedWindows(t *testing.T) { //nolint:paralleltest // mutates package-level tokenElevated
	// Exercise the real token query once; its value depends on the runner.
	_ = isElevated()

	orig := tokenElevated
	t.Cleanup(func() { tokenElevated = orig })
	tokenElevated = func() bool { return true }
	if !isElevated() {
		t.Error("expected elevated")
	}
}
