//go:build integration

package containers

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// TestIntegrationDocker runs a real container publishing a random host port
// and checks that the engine API and a real socket listing attribute the port
// to it. It needs a Docker engine running Linux containers (e.g. GitHub's
// Ubuntu runners or Docker Desktop) and skips otherwise.
func TestIntegrationDocker(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not installed")
	}
	out, err := exec.Command("docker", "info", "--format", "{{.OSType}}").Output()
	if err != nil || strings.TrimSpace(string(out)) != "linux" {
		t.Skipf("no Linux-container Docker engine available (%v, %q)", err, out)
	}
	engine, ok := Discover(os.Getenv)
	if !ok {
		t.Skip("docker works but no engine endpoint was discovered")
	}

	name := "portcheck-it-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	run := exec.Command("docker", "run", "-d", "--rm", "--name", name, "-p", "127.0.0.1::80", "busybox:1.36", "httpd", "-f", "-p", "80")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })

	portOut, err := exec.Command("docker", "port", name, "80/tcp").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	// "127.0.0.1:49153"
	_, portStr, _ := strings.Cut(strings.TrimSpace(strings.Split(string(portOut), "\n")[0]), ":")
	hostPort, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse docker port output %q: %v", portOut, err)
	}

	pubs, err := engine.Published(t.Context())
	if err != nil {
		t.Fatalf("Published via %s: %v", engine.Endpoint(), err)
	}
	found := false
	for _, p := range pubs {
		if p.Container.Name == name && int(p.HostPort) == hostPort && p.Proto == inspect.TCP && p.ContainerPort == 80 {
			found = true
		}
	}
	if !found {
		t.Fatalf("container %s with host port %d not in %+v", name, hostPort, pubs)
	}

	// A real system listing (process lookup + enrichment) shows the container.
	in := inspect.NewSystem(inspect.WithEnricher(engine))
	var res *inspect.Result
	deadline := time.Now().Add(10 * time.Second)
	for {
		res, err = in.List(t.Context(), inspect.Filter{Ports: portspec.Single(uint16(hostPort)), ListeningOnly: true})
		if err == nil && len(res.Sockets) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil || len(res.Sockets) == 0 {
		t.Fatalf("no listening socket on published port %d: %v", hostPort, err)
	}
	for _, s := range res.Sockets {
		if s.Container == nil || s.Container.Name != name || s.Container.Image != "busybox:1.36" {
			t.Errorf("socket %v not attributed to %s: %+v", s.Local, name, s.Container)
		}
	}
}
