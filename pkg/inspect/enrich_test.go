package inspect

import (
	"context"
	"errors"
	"testing"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// labelEnricher marks sockets on a port as belonging to a container.
type labelEnricher struct {
	port        uint16
	container   *Container
	err         error
	calls       int
	hadDeadline bool
}

func (e *labelEnricher) Enrich(ctx context.Context, socks []Socket) error {
	e.calls++
	_, e.hadDeadline = ctx.Deadline()
	for i := range socks {
		if socks[i].Port() == e.port {
			socks[i].Container = e.container
		}
	}
	return e.err
}

var webContainer = &Container{ID: "abc123", Name: "web", Image: "nginx:alpine", Runtime: "docker"}

func TestEnricherApplied(t *testing.T) {
	t.Parallel()
	e := &labelEnricher{port: 5432, container: webContainer}
	res, err := New(newFake(), WithEnricher(e)).List(t.Context(), Filter{Ports: portspec.Single(5432)})
	if err != nil {
		t.Fatal(err)
	}
	if e.calls != 1 || !e.hadDeadline {
		t.Errorf("calls=%d deadline=%v", e.calls, e.hadDeadline)
	}
	if len(res.Sockets) != 1 || res.Sockets[0].Container != webContainer {
		t.Errorf("sockets = %+v", res.Sockets)
	}
}

func TestEnricherErrorIgnored(t *testing.T) {
	t.Parallel()
	e := &labelEnricher{port: 5432, container: webContainer, err: errors.New("docker down")}
	res, err := New(newFake(), WithEnricher(e)).List(t.Context(), Filter{})
	if err != nil || len(res.Sockets) == 0 {
		t.Fatalf("List = %v, %v; enrichment errors must not fail listing", res, err)
	}
}

func TestEnricherSkippedWhenNothingListed(t *testing.T) {
	t.Parallel()
	e := &labelEnricher{}
	if _, err := New(&FakeProvider{}, WithEnricher(e)).List(t.Context(), Filter{}); err != nil {
		t.Fatal(err)
	}
	if e.calls != 0 {
		t.Errorf("enricher called %d times with no sockets", e.calls)
	}
}

func TestWithEnricherNilIgnored(t *testing.T) {
	t.Parallel()
	if in := New(nil, WithEnricher(nil)); len(in.enrichers) != 0 {
		t.Error("nil enricher was added")
	}
}

func TestNameFilterMatchesContainer(t *testing.T) {
	t.Parallel()
	e := &labelEnricher{port: 5432, container: webContainer}
	in := New(newFake(), WithEnricher(e))
	res, err := in.List(t.Context(), Filter{Name: "WE*"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sockets) != 1 || res.Sockets[0].Port() != 5432 {
		t.Errorf("container name filter = %+v", rawOf(res.Sockets))
	}
	// Process names still match on enriched sockets.
	res, _ = in.List(t.Context(), Filter{Name: "postgres"})
	if len(res.Sockets) != 1 {
		t.Errorf("process name filter = %+v", rawOf(res.Sockets))
	}
}

func TestNameHelpers(t *testing.T) {
	t.Parallel()
	if processName(nil) != "" || containerName(nil) != "" {
		t.Error("nil helpers must return empty names")
	}
	if processName(nodeProc) != "node" || containerName(webContainer) != "web" {
		t.Error("helpers returned wrong names")
	}
}
