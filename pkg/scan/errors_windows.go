//go:build windows

package scan

import "syscall"

// WSAECONNREFUSED; syscall.ECONNREFUSED is not what Winsock returns.
const errConnRefused = syscall.Errno(10061)
