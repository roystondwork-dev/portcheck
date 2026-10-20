//go:build windows

package cli

import "os"

// Windows has no signals to relay: the console delivers Ctrl+C to the child
// directly because it shares the console.
var forwardedSignals []os.Signal

// signalExitCode is never reached on Windows: exit codes are always set.
func signalExitCode(*os.ProcessState) int { return ExitRuntime }
