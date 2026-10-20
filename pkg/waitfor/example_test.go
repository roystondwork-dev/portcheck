package waitfor_test

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/waitfor"
)

// Wait up to 30 seconds for PostgreSQL to accept connections.
func ExamplePoll() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	check := waitfor.DialCheck(&net.Dialer{}, "tcp", "localhost:5432", time.Second)
	r := waitfor.Poll(ctx, 250*time.Millisecond, check)
	if r.Ready {
		fmt.Printf("ready after %s (%d attempts)\n", r.Waited.Round(time.Millisecond), r.Attempts)
	} else {
		fmt.Println("gave up:", ctx.Err())
	}
}
