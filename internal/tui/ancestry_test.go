package tui

import (
	"strings"
	"testing"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

func TestSelectedDetailAndConfirm(t *testing.T) {
	t.Parallel()
	dev := inspect.Socket{
		RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("127.0.0.1:3000"), State: inspect.StateListen, PID: 100},
		Process: &inspect.ProcessInfo{PID: 100, Name: "node", User: "alice", Cmdline: "node server.js",
			Ancestors: []inspect.ProcessRef{{PID: 90, Name: "npm"}, {PID: 80, Name: "zsh"}}},
	}
	onlyCmd := inspect.Socket{
		RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("127.0.0.1:4000"), State: inspect.StateListen, PID: 200},
		Process:   &inspect.ProcessInfo{PID: 200, Name: "ruby", Cmdline: "rails s"},
	}
	m, _, _ := newModel(t, []inspect.Socket{sHidden, dev, onlyCmd}, true)

	// Hidden owner selected: the privilege hint is shown instead.
	if !strings.Contains(screen(m), "some owners are hidden") {
		t.Errorf("hint missing:\n%s", screen(m))
	}
	m = press(t, m, "down")
	if !strings.Contains(screen(m), "started by npm (90) ← zsh (80) · node server.js") {
		t.Errorf("detail missing:\n%s", screen(m))
	}
	m = press(t, m, "down")
	if !strings.Contains(screen(m), "rails s") || strings.Contains(screen(m), "started by") {
		t.Errorf("command-only detail wrong:\n%s", screen(m))
	}

	m = press(t, m, "up", "x")
	if !strings.Contains(screen(m), "Stop node (pid 100) listening on 3000, started by npm (90) ← zsh (80)? [y/N]") {
		t.Errorf("confirm text:\n%s", screen(m))
	}
}
