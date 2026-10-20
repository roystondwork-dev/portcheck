// Package waitfor polls a condition until it holds or a context ends. It backs
// `portcheck wait`, e.g. waiting for a database to accept connections in CI.
package waitfor

import (
	"context"
	"net"
	"time"
)

// Check reports whether the awaited condition holds. It should return quickly
// and respect ctx.
type Check func(ctx context.Context) bool

// Result describes how a Poll ended.
type Result struct {
	// Ready is true if the condition held before ctx ended.
	Ready bool
	// Attempts is how many times the check ran.
	Attempts int
	// Waited is the time from the first attempt until the condition held or ctx ended.
	Waited time.Duration
}

// Poll runs check immediately and then every interval until it returns true
// or ctx is done. Inspect ctx.Err() to tell a timeout (DeadlineExceeded) from
// a cancellation. interval < 1ms is treated as 1ms.
func Poll(ctx context.Context, interval time.Duration, check Check) Result {
	interval = max(interval, time.Millisecond)
	start := time.Now()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var r Result
	for {
		r.Attempts++
		if check(ctx) {
			r.Ready = true
			r.Waited = time.Since(start)
			return r
		}
		select {
		case <-ctx.Done():
			r.Waited = time.Since(start)
			return r
		case <-ticker.C:
		}
	}
}

// Dialer opens connections; *net.Dialer implements it.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// DialCheck returns a Check that succeeds when a connection to address can be
// opened. Each attempt is limited to attemptTimeout (if > 0), so a silently
// dropped connection doesn't consume the whole wait.
func DialCheck(d Dialer, network, address string, attemptTimeout time.Duration) Check {
	return func(ctx context.Context) bool {
		if attemptTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, attemptTimeout)
			defer cancel()
		}
		conn, err := d.DialContext(ctx, network, address)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}
}
