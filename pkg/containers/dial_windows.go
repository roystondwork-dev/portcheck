//go:build windows

package containers

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

func dialPipe(ctx context.Context, path string) (net.Conn, error) {
	return winio.DialPipeContext(ctx, path)
}
