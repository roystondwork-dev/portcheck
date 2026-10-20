//go:build !windows

package cli_test

import "syscall"

func killSelf() int {
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGTERM)
	select {} // wait for the signal to terminate us
}
