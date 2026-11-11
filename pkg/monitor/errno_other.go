//go:build !windows

package monitor

import "syscall"

var (
	errRefused     = []error{syscall.ECONNREFUSED}
	errReset       = []error{syscall.ECONNRESET}
	errUnreachable = []error{syscall.EHOSTUNREACH, syscall.ENETUNREACH}
)
