package inspect

import (
	"runtime"
	"strings"
	"testing"
)

func TestPrivilegeHint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		goos     string
		elevated bool
		contains string
	}{
		{"darwin", true, ""},
		{"darwin", false, "sudo"},
		{"linux", false, "sudo"},
		{"freebsd", false, "sudo"},
		{"windows", false, "Administrator"},
		{"windows", true, ""},
	}
	for _, tt := range tests {
		got := privilegeHint(tt.goos, tt.elevated)
		if tt.contains == "" && got != "" {
			t.Errorf("privilegeHint(%s, %v) = %q, want empty", tt.goos, tt.elevated, got)
		}
		if !strings.Contains(got, tt.contains) {
			t.Errorf("privilegeHint(%s, %v) = %q, want it to mention %q", tt.goos, tt.elevated, got, tt.contains)
		}
	}
}

func TestPrivilegeHintMatchesCurrentProcess(t *testing.T) {
	t.Parallel()
	if got, want := PrivilegeHint(), privilegeHint(runtime.GOOS, Elevated()); got != want {
		t.Errorf("PrivilegeHint() = %q, want %q", got, want)
	}
}
