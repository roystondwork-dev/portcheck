//go:build darwin

package inspect

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestOpenLibproc(t *testing.T) {
	t.Parallel()
	if _, err := openLibproc("/nonexistent/libSystem.dylib"); err == nil {
		t.Error("loading a missing library must fail")
	}
	if lp, err := loadLibproc(); err != nil || lp == nil {
		t.Fatalf("loadLibproc = %v, %v", lp, err)
	}
}

func TestNativeSocketsLoadError(t *testing.T) {
	t.Parallel()
	boom := errors.New("no libSystem")
	if _, err := nativeSocketsWith(t.Context(), func() (*purelibproc, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Errorf("error = %v", err)
	}
}

func TestNativeSocketsFindsOwnListener(t *testing.T) {
	t.Parallel()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := uint16(l.Addr().(*net.TCPAddr).Port)

	socks, err := nativeSockets(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range socks {
		if s.Proto == TCP && s.Local.Port() == port && s.State == StateListen && s.PID == int32(os.Getpid()) {
			return
		}
	}
	t.Errorf("own listener on %d not found among %d sockets", port, len(socks))
}

func TestPurelibprocErrors(t *testing.T) {
	t.Parallel()
	failing := &purelibproc{
		listAllPids: func(unsafe.Pointer, int32) int32 { return -1 },
		pidInfo:     func(int32, int32, uint64, unsafe.Pointer, int32) int32 { return 0 },
		pidFDInfo:   func(int32, int32, int32, unsafe.Pointer, int32) int32 { return 10 },
	}
	if _, err := failing.listPIDs(); err == nil {
		t.Error("listPIDs: expected error")
	}
	if _, err := failing.listFDs(1); err == nil {
		t.Error("listFDs: expected error")
	}
	if _, err := failing.socketInfo(1, 2); err == nil {
		t.Error("socketInfo: short read must fail")
	}

	// The process count succeeds but the listing fails.
	calls := 0
	secondFails := &purelibproc{listAllPids: func(unsafe.Pointer, int32) int32 {
		calls++
		if calls == 1 {
			return 3
		}
		return 0
	}}
	if _, err := secondFails.listPIDs(); err == nil {
		t.Error("listPIDs: expected error from the second call")
	}
}

func TestPurelibprocListings(t *testing.T) {
	t.Parallel()
	lp := &purelibproc{
		listAllPids: func(buf unsafe.Pointer, size int32) int32 {
			if buf == nil {
				return 2
			}
			pids := unsafe.Slice((*int32)(buf), size/4)
			pids[0], pids[1] = 11, 22
			return 2
		},
	}
	pids, err := lp.listPIDs()
	if err != nil || fmt.Sprint(pids) != "[11 22]" {
		t.Errorf("listPIDs = %v, %v", pids, err)
	}

	// Buffer growth: a full buffer means "maybe more", so listFDs retries larger.
	var sizes []int32
	lp.pidInfo = func(_, _ int32, _ uint64, buf unsafe.Pointer, size int32) int32 {
		sizes = append(sizes, size)
		if len(sizes) == 1 {
			return size
		}
		entry := unsafe.Slice((*int32)(buf), 2)
		entry[0], entry[1] = 9, proxFDTypeSocket
		return procFDInfoSize
	}
	fds, err := lp.listFDs(1)
	if err != nil || len(fds) != 1 || fds[0] != (procFD{9, proxFDTypeSocket}) || len(sizes) != 2 || sizes[1] != 4*sizes[0] {
		t.Errorf("listFDs = %v, %v (sizes %v)", fds, err, sizes)
	}

	lp.pidInfo = func(_, _ int32, _ uint64, _ unsafe.Pointer, size int32) int32 { return size } // never fits
	if _, err := lp.listFDs(1); err == nil || !strings.Contains(err.Error(), "too many descriptors") {
		t.Errorf("listFDs cap error = %v", err)
	}

	lp.pidFDInfo = func(int32, int32, int32, unsafe.Pointer, int32) int32 { return socketFDInfoSize }
	if b, err := lp.socketInfo(1, 2); err != nil || len(b) != socketFDInfoSize {
		t.Errorf("socketInfo = %d bytes, %v", len(b), err)
	}
}

// TestLibprocLayoutMatchesSDK compiles static assertions of the struct offsets
// used by parseSocketFDInfo against the macOS SDK, for both architectures.
func TestLibprocLayoutMatchesSDK(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
	src := fmt.Sprintf(`#include <stddef.h>
#include <libproc.h>
#include <sys/proc_info.h>
_Static_assert(PROC_PIDLISTFDS == %d, "PROC_PIDLISTFDS");
_Static_assert(PROC_PIDFDSOCKETINFO == %d, "PROC_PIDFDSOCKETINFO");
_Static_assert(PROX_FDTYPE_SOCKET == %d, "PROX_FDTYPE_SOCKET");
_Static_assert(sizeof(struct proc_fdinfo) == %d, "proc_fdinfo");
_Static_assert(sizeof(struct socket_fdinfo) == %d, "socket_fdinfo");
_Static_assert(offsetof(struct socket_fdinfo, psi) == %d, "psi");
_Static_assert(offsetof(struct socket_info, soi_protocol) == %d, "protocol");
_Static_assert(offsetof(struct socket_info, soi_family) == %d, "family");
_Static_assert(offsetof(struct socket_info, soi_kind) == %d, "kind");
_Static_assert(offsetof(struct socket_info, soi_proto) == %d, "proto");
_Static_assert(offsetof(struct in_sockinfo, insi_fport) == %d, "fport");
_Static_assert(offsetof(struct in_sockinfo, insi_lport) == %d, "lport");
_Static_assert(offsetof(struct in_sockinfo, insi_vflag) == %d, "vflag");
_Static_assert(offsetof(struct in_sockinfo, insi_faddr) == %d, "faddr");
_Static_assert(offsetof(struct in_sockinfo, insi_laddr) == %d, "laddr");
_Static_assert(offsetof(struct tcp_sockinfo, tcpsi_state) == %d, "state");
_Static_assert(SOCKINFO_IN == %d && SOCKINFO_TCP == %d && INI_IPV4 == %d, "kinds");
_Static_assert(AF_INET == %d && AF_INET6 == %d && IPPROTO_TCP == %d && IPPROTO_UDP == %d, "families");
_Static_assert(TSI_S_LISTEN == 1 && TSI_S_ESTABLISHED == 4 && TSI_S__CLOSE_WAIT == 5 && TSI_S_TIME_WAIT == 10, "states");
`, procPIDListFDs, procPIDFDSocketInfo, proxFDTypeSocket, procFDInfoSize, socketFDInfoSize, psiOffset,
		soiProtocolOffset, soiFamilyOffset, soiKindOffset, soiProtoOffset, insiFPortOffset, insiLPortOffset,
		insiVFlagOffset, insiFAddrOffset, insiLAddrOffset, tcpsiStateOffset, sockinfoIn, sockinfoTCP, iniIPv4,
		afInet, afInet6, ipprotoTCP, ipprotoUDP)
	f := filepath.Join(t.TempDir(), "layout.c")
	if err := os.WriteFile(f, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"arm64", "x86_64"} {
		if out, err := exec.Command("clang", "-arch", arch, "-fsyntax-only", f).CombinedOutput(); err != nil {
			t.Errorf("%s: layout differs from the SDK:\n%s", arch, out)
		}
	}
}
