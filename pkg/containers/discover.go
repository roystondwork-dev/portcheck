package containers

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Discover returns a Client for the container engine to use: $DOCKER_HOST if
// set, otherwise the first well-known local socket that exists. ok is false
// when no engine endpoint is found. getenv is usually os.Getenv.
func Discover(getenv func(string) string) (c *Client, ok bool) {
	return discover(runtime.GOOS, getenv, socketExists)
}

func discover(goos string, getenv func(string) string, exists func(string) bool) (*Client, bool) {
	if host := getenv("DOCKER_HOST"); host != "" {
		c, err := New(host)
		return c, err == nil
	}
	for _, ep := range candidates(goos, getenv) {
		path, isUnix := strings.CutPrefix(ep, "unix://")
		if isUnix && !exists(path) {
			continue
		}
		c, err := New(ep)
		return c, err == nil
	}
	return nil, false
}

// candidates lists default endpoints in order of preference.
func candidates(goos string, getenv func(string) string) []string {
	home := getenv("HOME")
	xdg := getenv("XDG_RUNTIME_DIR")
	unix := func(parts ...string) string { return "unix://" + filepath.ToSlash(filepath.Join(parts...)) }

	var eps []string
	switch goos {
	case "windows":
		eps = append(eps, "npipe:////./pipe/docker_engine", "npipe:////./pipe/podman-machine-default")
	case "darwin":
		if home != "" {
			eps = append(eps,
				unix(home, ".docker", "run", "docker.sock"),     // Docker Desktop
				unix(home, ".colima", "default", "docker.sock"), // Colima
				unix(home, ".orbstack", "run", "docker.sock"),   // OrbStack
				unix(home, ".local", "share", "containers", "podman", "machine", "podman.sock"),
			)
		}
		eps = append(eps, "unix:///var/run/docker.sock")
	default: // linux and other unix
		eps = append(eps, "unix:///var/run/docker.sock")
		if xdg != "" {
			eps = append(eps, unix(xdg, "docker.sock"), unix(xdg, "podman", "podman.sock")) // rootless
		}
		eps = append(eps, "unix:///run/podman/podman.sock")
	}
	return eps
}

func socketExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
