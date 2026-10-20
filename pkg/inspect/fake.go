package inspect

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
)

// FakeProvider is a deterministic, in-memory Provider for tests. It is
// exported so that programs built on this package can test against it too.
// The zero value has no sockets and no processes.
type FakeProvider struct {
	SocketList  []RawSocket
	SocketsErr  error
	Processes   map[int32]*ProcessInfo
	ProcessErrs map[int32]error

	processCalls atomic.Int64
}

var _ Provider = (*FakeProvider)(nil)

// Sockets returns a copy of SocketList, or SocketsErr if set.
func (f *FakeProvider) Sockets(ctx context.Context) ([]RawSocket, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("fake sockets: %w", err)
	}
	if f.SocketsErr != nil {
		return nil, f.SocketsErr
	}
	return slices.Clone(f.SocketList), nil
}

// Process returns a copy of Processes[pid], ProcessErrs[pid] if set, or an
// error wrapping ErrProcessNotFound.
func (f *FakeProvider) Process(ctx context.Context, pid int32) (*ProcessInfo, error) {
	f.processCalls.Add(1)
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("fake process %d: %w", pid, err)
	}
	if err, ok := f.ProcessErrs[pid]; ok {
		return nil, err
	}
	p, ok := f.Processes[pid]
	if !ok {
		return nil, fmt.Errorf("pid %d: %w", pid, ErrProcessNotFound)
	}
	cp := *p
	return &cp, nil
}

// ProcessCalls returns how many times Process has been called.
func (f *FakeProvider) ProcessCalls() int64 { return f.processCalls.Load() }
