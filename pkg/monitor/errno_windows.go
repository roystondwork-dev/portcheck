//go:build windows

package monitor

import "syscall"

// Windows reports socket errors as Winsock codes, which syscall's portable
// constants don't match.
var (
	errRefused     = []error{syscall.ECONNREFUSED, syscall.Errno(10061)}                                            // WSAECONNREFUSED
	errReset       = []error{syscall.ECONNRESET, syscall.Errno(10054)}                                              // WSAECONNRESET
	errUnreachable = []error{syscall.EHOSTUNREACH, syscall.ENETUNREACH, syscall.Errno(10065), syscall.Errno(10051)} // WSAEHOSTUNREACH, WSAENETUNREACH
)
