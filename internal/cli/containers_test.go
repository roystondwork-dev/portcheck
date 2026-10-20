package cli

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

var webCtr = &inspect.Container{ID: "b1308ab03f9f", Name: "web", Image: "nginx:alpine", Runtime: "docker"}

// containerLister returns fixed sockets regardless of filter.
func containerLister(socks ...inspect.Socket) SocketLister {
	return listerFunc(func(context.Context, inspect.Filter) (*inspect.Result, error) {
		return &inspect.Result{Sockets: socks}, nil
	})
}

func published(port string, ctr *inspect.Container, pid int32) inspect.Socket {
	return inspect.Socket{
		RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: netip.MustParseAddrPort(port), State: inspect.StateListen, PID: pid},
		Process:   &inspect.ProcessInfo{PID: pid, Name: "com.docker.backend", User: "alice"},
		Container: ctr,
	}
}

func TestKillContainerPort(t *testing.T) {
	t.Parallel()
	runCases(t, []runCase{
		{
			name: "only container ports", args: []string{"kill", "18080", "18443", "--yes"},
			setup: func(h *harness) {
				h.app.Lister = containerLister(
					published("[::]:18080", webCtr, 900),
					published("0.0.0.0:18080", webCtr, 900),
					published("127.0.0.1:18443", webCtr, 900),
					published("0.0.0.0:5353", &inspect.Container{ID: "abc", Image: "coredns", Runtime: "podman"}, 901),
				)
			},
			wantCode: ExitNegative,
			wantErr: []string{
				"Port 18080, 18443 is published by docker container \"web\" (nginx:alpine). Stop it there instead:\n  docker stop web",
				"published by podman container \"abc\" (coredns)", "podman stop abc",
			},
			notErr: []string{"Nothing is listening"},
			check: func(t *testing.T, h *harness) {
				if len(h.killer.killed) != 0 {
					t.Errorf("killed %v; container proxies must never be killed", h.killer.killed)
				}
			},
		},
		{
			name: "container and normal process", args: []string{"kill", "18080", "3000", "--yes"},
			setup: func(h *harness) {
				h.app.Lister = containerLister(published("[::]:18080", webCtr, 900), inspect.Socket{RawSocket: nodeSock, Process: &inspect.ProcessInfo{PID: 100, Name: "node"}})
			},
			wantOut: []string{"100", "terminated"}, wantErr: []string{"docker stop web"},
			check: func(t *testing.T, h *harness) {
				if len(h.killer.killed) != 1 || h.killer.killed[0] != 100 {
					t.Errorf("killed %v, want only the node process", h.killer.killed)
				}
			},
		},
		{
			name: "container without image", args: []string{"kill", "1"},
			setup: func(h *harness) {
				h.app.Lister = containerLister(published("0.0.0.0:1", &inspect.Container{ID: "x1", Name: "n", Runtime: "docker"}, 9))
			},
			wantCode: ExitNegative, wantErr: []string{`container "n" (unknown image)`},
		},
	})
}

func TestListShowsContainer(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.app.Lister = containerLister(published("[::]:18080", webCtr, 900))
	if code := h.run(t, "list", "-o", "csv"); code != ExitOK {
		t.Fatal(code)
	}
	if !strings.Contains(h.stdout.String(), "docker:web") {
		t.Errorf("container not shown:\n%s", h.stdout)
	}
}

func TestNewAppContainerDiscovery(t *testing.T) { //nolint:paralleltest // uses t.Setenv
	// Supported endpoint: an enricher is configured (listing still works even
	// though nothing listens there).
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:1")
	if _, err := NewApp().Lister.List(t.Context(), inspect.Filter{Ports: portspec.Single(1)}); err != nil {
		t.Errorf("List with unreachable engine: %v", err)
	}
	// Unsupported endpoint: discovery is skipped.
	t.Setenv("DOCKER_HOST", "ssh://nowhere")
	if NewApp().Lister == nil {
		t.Error("lister missing")
	}
}
