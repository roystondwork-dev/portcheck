package waitfor

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestPollReadyImmediately(t *testing.T) {
	t.Parallel()
	r := Poll(t.Context(), time.Hour, func(context.Context) bool { return true })
	if !r.Ready || r.Attempts != 1 {
		t.Errorf("Poll = %+v, want ready after 1 attempt", r)
	}
}

func TestPollReadyAfterRetries(t *testing.T) {
	t.Parallel()
	var n atomic.Int32
	r := Poll(t.Context(), time.Millisecond, func(context.Context) bool { return n.Add(1) == 4 })
	if !r.Ready || r.Attempts != 4 || r.Waited <= 0 {
		t.Errorf("Poll = %+v, want ready after 4 attempts", r)
	}
}

func TestPollTimeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	r := Poll(ctx, 5*time.Millisecond, func(context.Context) bool { return false })
	if r.Ready || r.Attempts < 2 || r.Waited < 25*time.Millisecond {
		t.Errorf("Poll = %+v, want not ready after several attempts", r)
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Errorf("ctx.Err() = %v", ctx.Err())
	}
}

func TestPollTinyIntervalIsClamped(t *testing.T) {
	t.Parallel()
	var n atomic.Int32
	r := Poll(t.Context(), 0, func(context.Context) bool { return n.Add(1) == 3 })
	if !r.Ready || r.Attempts != 3 {
		t.Errorf("Poll = %+v", r)
	}
}

type fakeDialer struct {
	err      error
	sawLimit atomic.Bool
}

func (d *fakeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" || address != "db:5432" {
		return nil, errors.New("unexpected target " + network + " " + address)
	}
	if _, ok := ctx.Deadline(); ok {
		d.sawLimit.Store(true)
	}
	if d.err != nil {
		return nil, d.err
	}
	c1, c2 := net.Pipe()
	_ = c2.Close()
	return c1, nil
}

func TestDialCheck(t *testing.T) {
	t.Parallel()
	ok := &fakeDialer{}
	if !DialCheck(ok, "tcp", "db:5432", time.Second)(t.Context()) {
		t.Error("open target reported not ready")
	}
	if !ok.sawLimit.Load() {
		t.Error("attempt timeout not applied")
	}

	refused := &fakeDialer{err: errors.New("connection refused")}
	if DialCheck(refused, "tcp", "db:5432", 0)(t.Context()) {
		t.Error("refused target reported ready")
	}
	if refused.sawLimit.Load() {
		t.Error("attempt timeout applied although 0")
	}
}

func TestDialCheckRealListener(t *testing.T) {
	t.Parallel()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	check := DialCheck(&net.Dialer{}, "tcp", addr, time.Second)
	if !check(t.Context()) {
		t.Error("listening port reported not ready")
	}
	l.Close()
	if check(t.Context()) {
		t.Error("closed port reported ready")
	}
}
