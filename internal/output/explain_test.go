package output

import (
	"net/netip"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

func TestExplainLines(t *testing.T) {
	t.Parallel()
	nodeSock := sockets[1] // node, pid 4242, started by npm ← zsh
	nodeSock6 := nodeSock
	nodeSock6.Local = netip.MustParseAddrPort("[::1]:3000")
	bare := inspect.Socket{RawSocket: inspect.RawSocket{PID: 77}}
	hidden := inspect.Socket{}
	ctr := sockets[len(sockets)-1] // docker container "web"
	status := func(hs ...inspect.Socket) freeport.Status {
		return freeport.Status{Port: 3000, Proto: inspect.TCP, Availability: freeport.InUse, Holders: hs}
	}
	for _, tc := range []struct {
		name    string
		st      freeport.Status
		runHint string
		want    []string
	}{
		{"process", status(nodeSock, nodeSock6), "", []string{
			"portcheck: port 3000 is held by node (pid 4242), started by npm (4200) ← zsh (900)",
			"portcheck: stop it: portcheck kill 3000",
		}},
		{"run hint and more holders", status(nodeSock, bare), "portcheck run --port 3000 --next -- vite --port {port}", []string{
			"portcheck: port 3000 is held by node (pid 4242), started by npm (4200) ← zsh (900) (and 1 more)",
			"portcheck: stop it: portcheck kill 3000  ·  or use a free port: portcheck run --port 3000 --next -- vite --port {port}",
		}},
		{"no process details", status(bare), "", []string{
			"portcheck: port 3000 is held by a process (pid 77)",
			"portcheck: stop it: portcheck kill 3000",
		}},
		{"hidden owner", status(hidden), "", []string{
			"portcheck: port 3000 is held by another user's process",
			"portcheck: see who: sudo portcheck 3000",
		}},
		{"no holders", status(), "", []string{
			"portcheck: port 3000 is held by another user's process",
			"portcheck: see who: sudo portcheck 3000",
		}},
		{"container", status(ctr), "", []string{
			`portcheck: port 3000 is published by docker container "web" (nginx:alpine)`,
			"portcheck: stop it: docker stop web",
		}},
	} {
		if diff := cmp.Diff(tc.want, ExplainLines(tc.st, tc.runHint)); diff != "" {
			t.Errorf("%s (-want +got):\n%s", tc.name, diff)
		}
	}
}

func TestExplainDoc(t *testing.T) {
	t.Parallel()
	d := ExplainDoc(ExplainJSON{Command: "false", ExitCode: 1}, nil, nil)
	data := d.Data.(ExplainJSON)
	if d.Kind != "explain" || d.Text != "" || data.Ports == nil || data.InUse == nil {
		t.Errorf("empty explain = %+v", d)
	}
}
