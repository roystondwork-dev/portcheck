//go:build !windows

package scan

import "syscall"

const errConnRefused = syscall.ECONNREFUSED
