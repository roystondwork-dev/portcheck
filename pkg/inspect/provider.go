package inspect

import (
	"context"
	"errors"
)

// Provider is the boundary between portcheck and the operating system.
type Provider interface {
	// Sockets returns all TCP and UDP sockets visible to the caller.
	Sockets(ctx context.Context) ([]RawSocket, error)
	// Process returns information about pid. It returns an error wrapping
	// ErrProcessNotFound if the process no longer exists, or ErrAccessDenied
	// if the caller may not inspect it.
	Process(ctx context.Context, pid int32) (*ProcessInfo, error)
}

// Errors returned (wrapped) by Provider implementations.
var (
	ErrProcessNotFound = errors.New("process not found")
	ErrAccessDenied    = errors.New("access denied")
)
