// Package containers attributes published ports to containers by asking a
// Docker-compatible Engine API (Docker, Docker Desktop, Colima, OrbStack,
// Podman) which host ports each running container publishes.
//
// Without this, a published port appears to belong to a proxy process such as
// docker-proxy or com.docker.backend. Client implements inspect.Enricher so
// listings show the container instead.
package containers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

// ErrUnsupportedEndpoint is returned by New for endpoints it cannot use.
var ErrUnsupportedEndpoint = errors.New("unsupported container engine endpoint")

// Port is a host port published by a container.
type Port struct {
	Proto         inspect.Proto
	HostIP        netip.Addr // invalid or unspecified means all addresses
	HostPort      uint16
	ContainerPort uint16
}

// Published pairs a published port with its container.
type Published struct {
	Port
	Container inspect.Container
}

// Client talks to a Docker-compatible Engine API.
type Client struct {
	endpoint string
	runtime  string
	baseURL  string
	http     *http.Client
}

var _ inspect.Enricher = (*Client)(nil)

// New returns a Client for endpoint, which uses Docker's DOCKER_HOST syntax:
// unix:///path/to/docker.sock, npipe:////./pipe/docker_engine or
// tcp://host:port (plain HTTP; TLS endpoints are not supported).
func New(endpoint string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %w", ErrUnsupportedEndpoint, endpoint, err)
	}
	c := &Client{endpoint: endpoint, runtime: runtimeFor(endpoint), baseURL: "http://container-engine"}
	transport := &http.Transport{}
	switch u.Scheme {
	case "unix":
		if u.Path == "" {
			return nil, fmt.Errorf("%w: %q: missing socket path", ErrUnsupportedEndpoint, endpoint)
		}
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", u.Path)
		}
	case "npipe":
		pipe := strings.ReplaceAll(u.Path, "/", `\`) // npipe:////./pipe/x -> \\.\pipe\x
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialPipe(ctx, pipe)
		}
	case "tcp", "http":
		if u.Host == "" {
			return nil, fmt.Errorf("%w: %q: missing host", ErrUnsupportedEndpoint, endpoint)
		}
		c.baseURL = "http://" + u.Host
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedEndpoint, endpoint)
	}
	c.http = &http.Client{Transport: transport}
	return c, nil
}

// Endpoint returns the endpoint the client was created with.
func (c *Client) Endpoint() string { return c.endpoint }

func runtimeFor(endpoint string) string {
	if strings.Contains(strings.ToLower(endpoint), "podman") {
		return "podman"
	}
	return "docker"
}

// apiContainer is the subset of GET /containers/json used here.
type apiContainer struct {
	ID    string   `json:"Id"`
	Names []string `json:"Names"`
	Image string   `json:"Image"`
	Ports []struct {
		IP          string `json:"IP"`
		PrivatePort uint16 `json:"PrivatePort"`
		PublicPort  uint16 `json:"PublicPort"`
		Type        string `json:"Type"`
	} `json:"Ports"`
}

// Published lists the host ports published by running containers.
func (c *Client) Published(ctx context.Context) ([]Published, error) {
	// The unversioned path is served by both Docker and Podman at their
	// current API version.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/containers/json", nil)
	if err != nil {
		return nil, fmt.Errorf("container engine request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("container engine at %s: %w", c.endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("container engine at %s: %s", c.endpoint, resp.Status)
	}
	var raw []apiContainer
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("container engine at %s: decode response: %w", c.endpoint, err)
	}
	return c.convert(raw), nil
}

func (c *Client) convert(raw []apiContainer) []Published {
	var out []Published
	for _, rc := range raw {
		ctr := inspect.Container{ID: shortID(rc.ID), Image: rc.Image, Runtime: c.runtime}
		if len(rc.Names) > 0 {
			ctr.Name = strings.TrimPrefix(rc.Names[0], "/")
		}
		for _, p := range rc.Ports {
			proto := inspect.Proto(strings.ToLower(p.Type))
			if p.PublicPort == 0 || (proto != inspect.TCP && proto != inspect.UDP) {
				continue // not published on the host, or a protocol we don't track
			}
			ip, _ := netip.ParseAddr(p.IP) // empty/invalid means all addresses
			out = append(out, Published{
				Port:      Port{Proto: proto, HostIP: ip.Unmap(), HostPort: p.PublicPort, ContainerPort: p.PrivatePort},
				Container: ctr,
			})
		}
	}
	return out
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// Enrich sets Container on listening sockets whose port a container publishes.
func (c *Client) Enrich(ctx context.Context, sockets []inspect.Socket) error {
	pubs, err := c.Published(ctx)
	if err != nil {
		return err
	}
	for i := range sockets {
		for _, p := range pubs {
			if p.matches(sockets[i]) {
				ctr := p.Container
				sockets[i].Container = &ctr
				break
			}
		}
	}
	return nil
}

func (p Published) matches(s inspect.Socket) bool {
	if !s.IsListening() || s.Proto != p.Proto || s.Port() != p.HostPort {
		return false
	}
	local := s.Local.Addr().Unmap()
	// Engines publish 0.0.0.0 and :: separately while one dual-stack socket
	// may hold both, so any wildcard on either side matches.
	return !p.HostIP.IsValid() || p.HostIP.IsUnspecified() || local.IsUnspecified() || p.HostIP == local
}
