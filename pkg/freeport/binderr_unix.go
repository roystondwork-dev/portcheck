//go:build !windows

package freeport

import "syscall"

const (
	errAddrInUse    = syscall.EADDRINUSE
	errAddrNotAvail = syscall.EADDRNOTAVAIL
	errAFNoSupport  = syscall.EAFNOSUPPORT
	errAccess       = syscall.EACCES
)
