package tui

import (
	"strings"
	"testing"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

var sWeb = inspect.Socket{
	RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("[::]:18080"), State: inspect.StateListen, PID: 900},
	Process:   &inspect.ProcessInfo{PID: 900, Name: "com.docker.backend", User: "alice"},
	Container: &inspect.Container{ID: "b1308ab03f9f", Name: "web", Image: "nginx:alpine", Runtime: "docker"},
}

func TestContainerRow(t *testing.T) {
	t.Parallel()
	m, _, k := newModel(t, append([]inspect.Socket{sWeb}, fixture...), false)
	if !strings.Contains(screen(m), "docker:web") || strings.Contains(screen(m), "com.docker.backend") {
		t.Errorf("container not shown in place of the proxy:\n%s", screen(m))
	}

	// Searchable by container name and image.
	m = typeText(t, press(t, m, "/"), "nginx")
	if len(m.rows) != 1 || m.rows[0].Container == nil {
		t.Fatalf("image search rows = %v", ports(m))
	}

	// Kill shows a docker stop hint and never asks to kill the proxy.
	m = press(t, m, "enter", "x")
	if m.mode != modeNormal || k.pid != 0 {
		t.Fatalf("mode=%v killed=%d", m.mode, k.pid)
	}
	if !strings.Contains(screen(m), `port 18080 is published by docker container "web"; stop it with: docker stop web`) {
		t.Errorf("hint missing:\n%s", screen(m))
	}
}

func TestContainerLabelAndSort(t *testing.T) {
	t.Parallel()
	if got := containerLabel(&inspect.Container{ID: "abc", Runtime: "podman"}); got != "podman:abc" {
		t.Errorf("label = %q", got)
	}
	if processName(sWeb) != "docker:web" || processName(sNode) != "node" || processName(sHidden) != "" {
		t.Error("processName must prefer the container label")
	}
	unnamed := sWeb
	unnamed.Container = &inspect.Container{ID: "abc123", Runtime: "docker"}
	m, _, _ := newModel(t, []inspect.Socket{unnamed}, false)
	m = press(t, m, "x")
	if !strings.Contains(m.status, `container "abc123"`) {
		t.Errorf("unnamed container status = %q", m.status)
	}
}
