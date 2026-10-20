package containers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func env(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }

func TestCandidates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		goos string
		env  map[string]string
		want []string
	}{
		{"darwin", map[string]string{"HOME": "/Users/a"}, []string{
			"unix:///Users/a/.docker/run/docker.sock",
			"unix:///Users/a/.colima/default/docker.sock",
			"unix:///Users/a/.orbstack/run/docker.sock",
			"unix:///Users/a/.local/share/containers/podman/machine/podman.sock",
			"unix:///var/run/docker.sock",
		}},
		{"darwin", nil, []string{"unix:///var/run/docker.sock"}},
		{"linux", map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}, []string{
			"unix:///var/run/docker.sock",
			"unix:///run/user/1000/docker.sock",
			"unix:///run/user/1000/podman/podman.sock",
			"unix:///run/podman/podman.sock",
		}},
		{"linux", nil, []string{"unix:///var/run/docker.sock", "unix:///run/podman/podman.sock"}},
		{"windows", nil, []string{"npipe:////./pipe/docker_engine", "npipe:////./pipe/podman-machine-default"}},
	}
	for _, tt := range tests {
		if d := cmp.Diff(tt.want, candidates(tt.goos, env(tt.env))); d != "" {
			t.Errorf("%s %v (-want +got):\n%s", tt.goos, tt.env, d)
		}
	}
}

func TestDiscover(t *testing.T) {
	t.Parallel()
	none := func(string) bool { return false }
	only := func(path string) func(string) bool { return func(p string) bool { return p == path } }

	c, ok := discover("linux", env(map[string]string{"DOCKER_HOST": "tcp://10.0.0.2:2375"}), none)
	if !ok || c.Endpoint() != "tcp://10.0.0.2:2375" {
		t.Errorf("DOCKER_HOST: %v %v", c, ok)
	}
	if _, ok := discover("linux", env(map[string]string{"DOCKER_HOST": "ssh://host"}), none); ok {
		t.Error("unsupported DOCKER_HOST must not be used")
	}
	c, ok = discover("linux", env(map[string]string{"XDG_RUNTIME_DIR": "/run/user/1"}), only("/run/user/1/docker.sock"))
	if !ok || c.Endpoint() != "unix:///run/user/1/docker.sock" {
		t.Errorf("rootless: %v %v", c, ok)
	}
	if _, ok := discover("linux", env(nil), none); ok {
		t.Error("found an engine where none exists")
	}
	c, ok = discover("windows", env(nil), none)
	if !ok || c.Endpoint() != "npipe:////./pipe/docker_engine" {
		t.Errorf("windows: %v %v", c, ok)
	}
}

func TestDiscoverUsesRealOS(t *testing.T) {
	t.Parallel()
	c, ok := Discover(env(map[string]string{"DOCKER_HOST": "tcp://127.0.0.1:1"}))
	if !ok || c.Endpoint() != "tcp://127.0.0.1:1" {
		t.Errorf("Discover = %v, %v", c, ok)
	}
}

func TestSocketExists(t *testing.T) {
	t.Parallel()
	f := filepath.Join(t.TempDir(), "x")
	if socketExists(f) {
		t.Error("missing file reported as existing")
	}
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !socketExists(f) {
		t.Error("existing file reported as missing")
	}
}
