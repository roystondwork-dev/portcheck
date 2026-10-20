package inspect

import "runtime"

// Elevated reports whether the current process runs with root/Administrator
// privileges.
func Elevated() bool { return isElevated() }

// PrivilegeHint returns advice on seeing more sockets or process details, or
// "" when the process is already elevated.
func PrivilegeHint() string { return privilegeHint(runtime.GOOS, Elevated()) }

func privilegeHint(goos string, elevated bool) string {
	if elevated {
		return ""
	}
	switch goos {
	case "darwin":
		return "Only your own processes are visible. Re-run with sudo to see sockets owned by other users."
	case "windows":
		return "Some process details are hidden. Re-run from an elevated (Run as Administrator) terminal to see them."
	default:
		return "Some process details are hidden. Re-run with sudo to see owners of all sockets."
	}
}
