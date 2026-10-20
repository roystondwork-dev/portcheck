//go:build darwin

package inspect

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Buffers the kernel writes into are passed as unsafe.Pointer, never uintptr:
// purego treats uintptr as a plain integer, which doesn't keep Go memory alive
// or pinned for the call.
type purelibproc struct {
	listAllPids func(buf unsafe.Pointer, size int32) int32
	pidInfo     func(pid, flavor int32, arg uint64, buf unsafe.Pointer, size int32) int32
	pidFDInfo   func(pid, fd, flavor int32, buf unsafe.Pointer, size int32) int32
}

var (
	libprocOnce sync.Once
	libprocImpl *purelibproc
	libprocErr  error
)

const libSystemPath = "/usr/lib/libSystem.B.dylib"

func loadLibproc() (*purelibproc, error) {
	libprocOnce.Do(func() { libprocImpl, libprocErr = openLibproc(libSystemPath) })
	return libprocImpl, libprocErr
}

func openLibproc(path string) (*purelibproc, error) {
	h, err := purego.Dlopen(path, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	lp := &purelibproc{}
	purego.RegisterLibFunc(&lp.listAllPids, h, "proc_listallpids")
	purego.RegisterLibFunc(&lp.pidInfo, h, "proc_pidinfo")
	purego.RegisterLibFunc(&lp.pidFDInfo, h, "proc_pidfdinfo")
	return lp, nil
}

// nativeSockets lists sockets with libproc, without running lsof.
var nativeSockets = func(ctx context.Context) ([]RawSocket, error) {
	return nativeSocketsWith(ctx, loadLibproc)
}

func nativeSocketsWith(ctx context.Context, load func() (*purelibproc, error)) ([]RawSocket, error) {
	lp, err := load()
	if err != nil {
		return nil, err
	}
	return libprocSockets(ctx, lp)
}

var errLibproc = errors.New("libproc call failed")

func (lp *purelibproc) listPIDs() ([]int32, error) {
	n := lp.listAllPids(nil, 0) // number of processes
	if n <= 0 {
		return nil, fmt.Errorf("proc_listallpids: %w", errLibproc)
	}
	buf := make([]int32, n+64)                                        // room for processes started meanwhile
	got := lp.listAllPids(unsafe.Pointer(&buf[0]), int32(len(buf)*4)) //nolint:gosec // audited: Go buffer of exactly the size passed, filled by the kernel
	if got <= 0 {
		return nil, fmt.Errorf("proc_listallpids: %w", errLibproc)
	}
	return buf[:min(int(got), len(buf))], nil
}

func (lp *purelibproc) listFDs(pid int32) ([]procFD, error) {
	for size := 64 * procFDInfoSize; size <= 1<<20; size *= 4 {
		buf := make([]byte, size)
		n := lp.pidInfo(pid, procPIDListFDs, 0, unsafe.Pointer(&buf[0]), int32(size)) //nolint:gosec // audited: Go buffer of exactly the size passed, filled by the kernel
		if n <= 0 {
			return nil, fmt.Errorf("proc_pidinfo(%d): %w", pid, errLibproc)
		}
		if int(n) < size { // the buffer was big enough
			return parseProcFDs(buf[:n]), nil
		}
	}
	return nil, fmt.Errorf("proc_pidinfo(%d): too many descriptors: %w", pid, errLibproc)
}

func (lp *purelibproc) socketInfo(pid, fd int32) ([]byte, error) {
	buf := make([]byte, socketFDInfoSize)
	n := lp.pidFDInfo(pid, fd, procPIDFDSocketInfo, unsafe.Pointer(&buf[0]), socketFDInfoSize) //nolint:gosec // audited: Go buffer of exactly the size passed, filled by the kernel
	if n != socketFDInfoSize {
		return nil, fmt.Errorf("proc_pidfdinfo(%d, %d): %w", pid, fd, errLibproc)
	}
	return buf, nil
}
