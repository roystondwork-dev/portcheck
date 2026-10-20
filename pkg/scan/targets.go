package scan

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// DefaultMaxHosts bounds how many hosts Expand produces unless told otherwise.
const DefaultMaxHosts = 1024

// Errors returned (wrapped) by Expand.
var (
	ErrBadTarget    = errors.New("invalid target")
	ErrTooManyHosts = errors.New("too many hosts")
	ErrResolve      = errors.New("cannot resolve host")
)

// Target is one host to scan.
type Target struct {
	// Host is the name as given by the user, or the address for IP/CIDR targets.
	Host string
	Addr netip.Addr
}

// String returns "host (addr)" for hostnames and just the address otherwise.
func (t Target) String() string {
	if a := t.Addr.String(); t.Host != a {
		return t.Host + " (" + a + ")"
	}
	return t.Host
}

// Resolver resolves hostnames; *net.Resolver implements it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Expand turns user-supplied targets (hostnames, IPv4/IPv6 addresses, CIDR
// prefixes) into a de-duplicated list of at most maxHosts hosts, in the order
// given. Hostnames resolve to their first IPv4 address, or first IPv6 address
// if there is none. maxHosts < 1 means DefaultMaxHosts.
func Expand(ctx context.Context, r Resolver, specs []string, maxHosts int) ([]Target, error) {
	if maxHosts < 1 {
		maxHosts = DefaultMaxHosts
	}
	var out []Target
	seen := map[netip.Addr]bool{}
	add := func(t Target) error {
		if seen[t.Addr] {
			return nil
		}
		if len(out) >= maxHosts {
			return fmt.Errorf("%w: more than %d (raise the limit to scan more)", ErrTooManyHosts, maxHosts)
		}
		seen[t.Addr] = true
		out = append(out, t)
		return nil
	}

	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		switch kind := classifySpec(spec); kind {
		case specPrefix:
			if err := expandPrefix(spec, maxHosts, add); err != nil {
				return nil, err
			}
		case specAddr:
			a := netip.MustParseAddr(strings.Trim(spec, "[]")).Unmap()
			if err := add(Target{Host: a.String(), Addr: a}); err != nil {
				return nil, err
			}
		case specHost:
			a, err := resolve(ctx, r, spec)
			if err != nil {
				return nil, err
			}
			if err := add(Target{Host: spec, Addr: a}); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("%w: %q", ErrBadTarget, spec)
		}
	}
	return out, nil
}

type specKind int

const (
	specInvalid specKind = iota
	specPrefix
	specAddr
	specHost
)

func classifySpec(s string) specKind {
	if _, err := netip.ParsePrefix(s); err == nil {
		return specPrefix
	}
	if _, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
		return specAddr
	}
	if isHostname(s) {
		return specHost
	}
	return specInvalid
}

// isHostname accepts RFC 1123 style names (letters, digits, '-', '_', '.').
func isHostname(s string) bool {
	if s == "" || len(s) > 253 || strings.HasPrefix(s, "-") || strings.HasPrefix(s, ".") || strings.Contains(s, "..") {
		return false
	}
	return strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.") == ""
}

func expandPrefix(spec string, maxHosts int, add func(Target) error) error {
	p := netip.MustParsePrefix(spec).Masked()
	hostBits := p.Addr().BitLen() - p.Bits()
	// Reject huge prefixes before enumerating them.
	if hostBits >= 31 || 1<<hostBits > maxHosts {
		return fmt.Errorf("%w: %s has %s addresses, limit is %d", ErrTooManyHosts, spec, sizeString(hostBits), maxHosts)
	}
	for a := p.Addr(); p.Contains(a); a = a.Next() {
		if err := add(Target{Host: a.String(), Addr: a}); err != nil {
			return err
		}
	}
	return nil
}

func sizeString(hostBits int) string {
	if hostBits >= 31 {
		return fmt.Sprintf("2^%d", hostBits)
	}
	return strconv.Itoa(1 << hostBits)
}

func resolve(ctx context.Context, r Resolver, host string) (netip.Addr, error) {
	addrs, err := r.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("%w %q: %w", ErrResolve, host, err)
	}
	if len(addrs) == 0 {
		return netip.Addr{}, fmt.Errorf("%w %q: no addresses", ErrResolve, host)
	}
	for _, a := range addrs {
		if a.Unmap().Is4() {
			return a.Unmap(), nil
		}
	}
	return addrs[0], nil
}
