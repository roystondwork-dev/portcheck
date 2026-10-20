package cli

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/internal/tui"
)

type fakeTUI struct {
	called bool
	deps   tui.Deps
	in     io.Reader
	out    io.Writer
	err    error
}

func (f *fakeTUI) run(_ context.Context, deps tui.Deps, in io.Reader, out io.Writer, _ []string) error {
	f.called, f.deps, f.in, f.out = true, deps, in, out
	return f.err
}

func tuiHarness(stdinTTY, stdoutTTY bool, err error) (*harness, *fakeTUI) {
	h := newHarness()
	f := &fakeTUI{err: err}
	h.app.RunTUI = f.run
	h.app.StdinTTY, h.app.StdoutTTY = stdinTTY, stdoutTTY
	return h, f
}

func TestTUICommand(t *testing.T) {
	t.Parallel()
	h, f := tuiHarness(true, true, nil)
	if code := h.run(t, "tui"); code != ExitOK || !f.called {
		t.Fatalf("exit = %d, called = %v", code, f.called)
	}
	if f.deps.Interval != defaultTUIInterval || f.deps.Hint != "re-run with sudo" || f.deps.Lister == nil || f.deps.Killer == nil {
		t.Errorf("deps = %+v", f.deps)
	}
	if f.in != h.app.Stdin || f.out != h.app.Stdout {
		t.Error("TUI not wired to the app's streams")
	}

	h, f = tuiHarness(true, true, nil)
	h.run(t, "tui", "--interval", "500ms")
	if f.deps.Interval != 500*time.Millisecond {
		t.Errorf("interval = %v", f.deps.Interval)
	}
}

func TestTUIErrors(t *testing.T) {
	t.Parallel()
	runCases(t, []runCase{
		{name: "needs terminal", args: []string{"tui"}, setup: func(h *harness) { h.app.StdinTTY = true }, wantCode: ExitUsage, wantErr: []string{"needs an interactive terminal", "portcheck list"}},
		{name: "bad interval", args: []string{"tui", "--interval", "0s"}, wantCode: ExitUsage, wantErr: []string{"--interval must be positive"}},
		{name: "no args allowed", args: []string{"tui", "x"}, wantCode: ExitUsage},
	})

	h, _ := tuiHarness(true, true, context.Canceled)
	if code := h.run(t, "tui"); code != ExitInterrupted {
		t.Errorf("cancelled TUI exit = %d", code)
	}
	h, _ = tuiHarness(true, true, errors.New("tty broke"))
	if code := h.run(t, "tui"); code != ExitRuntime {
		t.Errorf("failed TUI exit = %d", code)
	}
}

func TestBareCommandOpensTUIOnlyInTerminal(t *testing.T) {
	t.Parallel()
	h, f := tuiHarness(true, true, nil)
	h.run(t)
	if !f.called || f.deps.Interval != defaultTUIInterval {
		t.Error("bare portcheck in a terminal must open the TUI")
	}
	for _, tty := range [][2]bool{{false, true}, {true, false}, {false, false}} {
		h, f := tuiHarness(tty[0], tty[1], nil)
		h.run(t)
		if f.called {
			t.Errorf("TUI opened with stdin TTY=%v stdout TTY=%v", tty[0], tty[1])
		}
	}
}
