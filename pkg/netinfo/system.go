package netinfo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Source is where the network facts come from; *System reads them from the
// operating system, and tests use fakes.
type Source interface {
	// Interfaces lists the interfaces that are up.
	Interfaces() ([]Interface, error)
	// Primary returns the local address used to reach the internet ("udp4" or
	// "udp6"). No packet is sent.
	Primary(network string) (netip.Addr, error)
	// Gateways lists the default gateways.
	Gateways(ctx context.Context) ([]netip.Addr, error)
	// DNSServers lists the configured DNS servers.
	DNSServers(ctx context.Context) ([]netip.Addr, error)
	// Neighbors lists the neighbour (ARP) cache.
	Neighbors(ctx context.Context) ([]Neighbor, error)
}

// ErrUnsupported is returned when this OS has no source for a fact.
var ErrUnsupported = errors.New("not supported on this operating system")

// source is one place a fact is read from: a file or a command's output
// (parsed with parse), or a function that reads it directly.
type source[T any] struct {
	file  string
	cmd   []string
	parse func(string) []T
	fn    func(context.Context) ([]T, error)
}

// System reads network facts from this machine. The zero value is not usable;
// use NewSystem.
type System struct {
	interfaces func() ([]net.Interface, error)
	addrs      func(net.Interface) ([]net.Addr, error)
	dial       func(network, address string) (net.Conn, error)
	readFile   func(string) ([]byte, error)
	run        func(ctx context.Context, name string, args ...string) ([]byte, error)

	gatewaySources  []source[netip.Addr]
	dnsSources      []source[netip.Addr]
	neighborSources []source[Neighbor]
}

// NewSystem returns a Source backed by this operating system.
func NewSystem() *System {
	return &System{
		interfaces:      net.Interfaces,
		addrs:           func(i net.Interface) ([]net.Addr, error) { return i.Addrs() },
		dial:            net.Dial,
		readFile:        os.ReadFile,
		run:             runCommand,
		gatewaySources:  gatewaySources,
		dnsSources:      dnsSources,
		neighborSources: neighborSources,
	}
}

// runCommand runs a command with the C locale, so its output can be parsed.
func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// name and args come from the fixed per-OS source tables, never from input.
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: fixed commands (sources_*.go)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

// Interfaces lists the interfaces that are up.
func (s *System) Interfaces() ([]Interface, error) {
	nis, err := s.interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}
	var out []Interface
	for _, ni := range nis {
		if ni.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := s.addrs(ni)
		if err != nil {
			continue // an interface that vanished while listing
		}
		out = append(out, fromNet(ni, addrs))
	}
	return out, nil
}

// primaryTargets are well-known anycast resolvers used only to pick a route:
// "dialing" UDP sends nothing.
var primaryTargets = map[string]string{"udp4": "1.1.1.1:53", "udp6": "[2606:4700:4700::1111]:53"}

// Primary returns the local address the OS would use to reach the internet.
func (s *System) Primary(network string) (netip.Addr, error) {
	target, ok := primaryTargets[network]
	if !ok {
		return netip.Addr{}, fmt.Errorf("primary address: unknown network %q", network)
	}
	c, err := s.dial(network, target)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("no %s route: %w", strings.TrimPrefix(network, "udp"), err)
	}
	defer c.Close()
	ap, err := netip.ParseAddrPort(c.LocalAddr().String())
	if err != nil {
		return netip.Addr{}, fmt.Errorf("primary address: %w", err)
	}
	return ap.Addr().Unmap(), nil
}

// Gateways lists the default gateways.
func (s *System) Gateways(ctx context.Context) ([]netip.Addr, error) {
	return gather(ctx, s, s.gatewaySources)
}

// DNSServers lists the configured DNS servers.
func (s *System) DNSServers(ctx context.Context) ([]netip.Addr, error) {
	return gather(ctx, s, s.dnsSources)
}

// Neighbors lists the neighbour (ARP) cache.
func (s *System) Neighbors(ctx context.Context) ([]Neighbor, error) {
	return gather(ctx, s, s.neighborSources)
}

// gather reads every source and returns what they found together. It fails
// only when every source failed.
func gather[T any](ctx context.Context, s *System, srcs []source[T]) ([]T, error) {
	if len(srcs) == 0 {
		return nil, ErrUnsupported
	}
	var out []T
	var firstErr error
	ok := false
	for _, src := range srcs {
		found, err := read(ctx, s, src)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ok = true
		out = append(out, found...)
	}
	if !ok {
		return nil, firstErr
	}
	return out, nil
}

// read reads one source.
func read[T any](ctx context.Context, s *System, src source[T]) ([]T, error) {
	var b []byte
	var err error
	switch {
	case src.fn != nil:
		return src.fn(ctx)
	case src.file != "":
		b, err = s.readFile(src.file)
	default:
		b, err = s.run(ctx, src.cmd[0], src.cmd[1:]...)
	}
	if err != nil {
		return nil, err
	}
	return src.parse(string(b)), nil
}

// PublicIPEndpoints are the only URLs portcheck contacts for the public
// address (ipify, https://www.ipify.org): IPv4, then IPv6.
var PublicIPEndpoints = []string{"https://api.ipify.org", "https://api6.ipify.org"}

// Getter fetches a URL's body; HTTPGet implements it.
type Getter func(ctx context.Context, url string) (string, error)

// HTTPGet fetches url with a 5 second timeout.
func HTTPGet(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("public address: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("public address: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("public address: %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", fmt.Errorf("public address: %w", err)
	}
	return string(b), nil
}

// PublicIPs asks ipify for this network's public addresses, IPv4 and IPv6.
// It is the only function in this package that contacts the internet. A
// network without IPv6 simply returns the IPv4 address; the error is set only
// when no address was found.
func PublicIPs(ctx context.Context, get Getter) ([]netip.Addr, error) {
	var out []netip.Addr
	var firstErr error
	for _, u := range PublicIPEndpoints {
		body, err := get(ctx, u)
		if err == nil {
			a, perr := netip.ParseAddr(strings.TrimSpace(body))
			if perr == nil {
				out = append(out, a.Unmap())
				continue
			}
			err = fmt.Errorf("public address: %s returned %q", u, strings.TrimSpace(body))
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if len(out) == 0 {
		return nil, firstErr
	}
	return out, nil
}
