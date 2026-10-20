package inspect

import (
	"context"
	"errors"
	"testing"
)

func TestFakeProvider(t *testing.T) {
	t.Parallel()
	fp := &FakeProvider{
		SocketList: []RawSocket{sNodeListen},
		Processes:  map[int32]*ProcessInfo{100: nodeProc},
	}

	got, err := fp.Sockets(t.Context())
	if err != nil || len(got) != 1 {
		t.Fatalf("Sockets = %v, %v", got, err)
	}
	got[0].PID = 999
	if fp.SocketList[0].PID != 100 {
		t.Error("Sockets must return a copy")
	}

	p, err := fp.Process(t.Context(), 100)
	if err != nil || p.Name != "node" {
		t.Fatalf("Process = %v, %v", p, err)
	}
	p.Name = "mutated"
	if nodeProc.Name != "node" {
		t.Error("Process must return a copy")
	}

	if _, err := fp.Process(t.Context(), 1); !errors.Is(err, ErrProcessNotFound) {
		t.Errorf("unknown pid error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := fp.Process(ctx, 100); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled Process error = %v", err)
	}
	if _, err := fp.Sockets(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled Sockets error = %v", err)
	}
	if fp.ProcessCalls() != 3 {
		t.Errorf("ProcessCalls = %d, want 3", fp.ProcessCalls())
	}

	var zero FakeProvider
	if s, err := zero.Sockets(t.Context()); err != nil || len(s) != 0 {
		t.Errorf("zero value Sockets = %v, %v", s, err)
	}
}
