package inspect

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/netip"
)

// This file enumerates sockets the way lsof does on macOS, via libproc:
// list processes, list each process's file descriptors, and read the socket
// details of each socket descriptor. The libSystem calls live behind the
// libproc interface (bound with purego in libproc_darwin.go) so that this
// logic is portable and unit-testable on every OS.

// Constants and struct layout from <sys/proc_info.h>. Offsets were taken from
// the macOS SDK and are identical on arm64 and x86_64; see
// TestLibprocLayoutMatchesSDK.
const (
	procPIDListFDs      = 1   // PROC_PIDLISTFDS
	procPIDFDSocketInfo = 3   // PROC_PIDFDSOCKETINFO
	proxFDTypeSocket    = 2   // PROX_FDTYPE_SOCKET
	procFDInfoSize      = 8   // sizeof(struct proc_fdinfo)
	socketFDInfoSize    = 792 // sizeof(struct socket_fdinfo)
	psiOffset           = 24  // socket_fdinfo.psi
	soiProtocolOffset   = 156
	soiFamilyOffset     = 160
	soiKindOffset       = 232
	soiProtoOffset      = 240 // union soi_proto: in_sockinfo / tcp_sockinfo
	insiFPortOffset     = 0
	insiLPortOffset     = 4
	insiVFlagOffset     = 24
	insiFAddrOffset     = 32
	insiLAddrOffset     = 48
	tcpsiStateOffset    = 80
	sockinfoIn          = 1 // SOCKINFO_IN (UDP and others)
	sockinfoTCP         = 2 // SOCKINFO_TCP
	iniIPv4             = 1 // INI_IPV4
	afInet              = 2
	afInet6             = 30
	ipprotoTCP          = 6
	ipprotoUDP          = 17
)

// tcpStates maps TSI_S_* values to state names (as lsof prints them).
var tcpStates = []State{
	"CLOSED", StateListen, "SYN_SENT", "SYN_RECEIVED", StateEstablished, StateCloseWait,
	"FIN_WAIT_1", "CLOSING", "LAST_ACK", "FIN_WAIT_2", StateTimeWait,
}

// procFD is one entry of PROC_PIDLISTFDS.
type procFD struct {
	fd  int32
	typ uint32
}

// libproc is the subset of libSystem's libproc API used here.
type libproc interface {
	listPIDs() ([]int32, error)
	listFDs(pid int32) ([]procFD, error)
	socketInfo(pid, fd int32) ([]byte, error) // a struct socket_fdinfo
}

// libprocSockets lists TCP and UDP sockets of every process lp can inspect.
// Processes and descriptors that can't be read (other users' processes
// without root, processes that just exited) are skipped, as lsof does.
func libprocSockets(ctx context.Context, lp libproc) ([]RawSocket, error) {
	pids, err := lp.listPIDs()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	var out []RawSocket
	for _, pid := range pids {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("list sockets: %w", err)
		}
		fds, err := lp.listFDs(pid)
		if err != nil {
			continue
		}
		for _, f := range fds {
			if f.typ != proxFDTypeSocket {
				continue
			}
			buf, err := lp.socketInfo(pid, f.fd)
			if err != nil {
				continue
			}
			if s, ok := parseSocketFDInfo(buf); ok {
				s.PID = pid
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// parseProcFDs decodes a PROC_PIDLISTFDS buffer of n bytes.
func parseProcFDs(buf []byte) []procFD {
	out := make([]procFD, 0, len(buf)/procFDInfoSize)
	for i := 0; i+procFDInfoSize <= len(buf); i += procFDInfoSize {
		out = append(out, procFD{
			fd:  int32(binary.LittleEndian.Uint32(buf[i:])),
			typ: binary.LittleEndian.Uint32(buf[i+4:]),
		})
	}
	return out
}

// parseSocketFDInfo decodes a struct socket_fdinfo, returning ok=false for
// anything but TCP/UDP over IPv4/IPv6.
func parseSocketFDInfo(b []byte) (RawSocket, bool) {
	if len(b) < socketFDInfoSize {
		return RawSocket{}, false
	}
	psi := b[psiOffset:]
	le := binary.LittleEndian
	family := le.Uint32(psi[soiFamilyOffset:])
	protocol := le.Uint32(psi[soiProtocolOffset:])
	kind := le.Uint32(psi[soiKindOffset:])
	if family != afInet && family != afInet6 {
		return RawSocket{}, false
	}

	var s RawSocket
	switch {
	case protocol == ipprotoTCP && kind == sockinfoTCP:
		s.Proto = TCP
	case protocol == ipprotoUDP && kind == sockinfoIn:
		s.Proto, s.State = UDP, StateNone
	default:
		return RawSocket{}, false
	}

	in := psi[soiProtoOffset:] // in_sockinfo (first member of tcp_sockinfo too)
	lport := netPort(in[insiLPortOffset:])
	if lport == 0 {
		return RawSocket{}, false // not bound to a port (lsof's "*:*"); nothing to report
	}
	v4 := family == afInet || in[insiVFlagOffset] == iniIPv4
	s.Local = netip.AddrPortFrom(sockAddr(in[insiLAddrOffset:insiLAddrOffset+16], v4), lport)
	if fport := netPort(in[insiFPortOffset:]); fport != 0 {
		s.Remote = netip.AddrPortFrom(sockAddr(in[insiFAddrOffset:insiFAddrOffset+16], v4), fport)
	}
	if s.Proto == TCP {
		st := le.Uint32(in[tcpsiStateOffset:])
		if int(st) < len(tcpStates) {
			s.State = tcpStates[st]
		} else {
			s.State = State(fmt.Sprintf("STATE_%d", st))
		}
	}
	return s, true
}

// netPort reads a port stored in network byte order in the low 16 bits of an int.
func netPort(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }

// sockAddr decodes an in4in6_addr union: IPv4 sits in the last 4 bytes.
func sockAddr(b []byte, v4 bool) netip.Addr {
	if v4 {
		return netip.AddrFrom4([4]byte(b[12:16]))
	}
	return netip.AddrFrom16([16]byte(b)).Unmap()
}
