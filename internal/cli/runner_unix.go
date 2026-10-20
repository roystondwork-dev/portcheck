//go:build !windows

package cli

import (
	"os"
	"syscall"
)

// forwardedSignals are relayed to the child. SIGINT is not: Ctrl+C already
// reaches the child directly because it shares the terminal's process group,
// and relaying it would deliver it twice.
var forwardedSignals = []os.Signal{syscall.SIGTERM, syscall.SIGHUP}

// signalExitCode returns 128+N for a process killed by signal N.
func signalExitCode(ps *os.ProcessState) int {
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ExitRuntime
}
