//go:build windows

package freeport

import "syscall"

// Winsock error codes; syscall.EADDRINUSE etc. are not what Windows returns.
const (
	errAccess       = syscall.Errno(10013) // WSAEACCES (e.g. excluded port range)
	errAFNoSupport  = syscall.Errno(10047) // WSAEAFNOSUPPORT
	errAddrInUse    = syscall.Errno(10048) // WSAEADDRINUSE
	errAddrNotAvail = syscall.Errno(10049) // WSAEADDRNOTAVAIL
)
