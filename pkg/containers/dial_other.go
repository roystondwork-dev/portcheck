//go:build !windows

package containers

import (
	"context"
	"errors"
	"net"
)

var errPipeUnsupported = errors.New("named pipes are only supported on Windows")

func dialPipe(context.Context, string) (net.Conn, error) { return nil, errPipeUnsupported }
