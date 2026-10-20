//go:build !darwin

package inspect

import "context"

// nativeSockets is only implemented on macOS; elsewhere gopsutil is used.
var nativeSockets func(ctx context.Context) ([]RawSocket, error)
