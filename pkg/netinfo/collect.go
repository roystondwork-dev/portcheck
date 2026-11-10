package netinfo

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
)

// Identity is this machine on the network.
type Identity struct {
	Interfaces []Interface
	// Primary4 and Primary6 are the local addresses used to reach the
	// internet; zero when there is no such route.
	Primary4, Primary6 netip.Addr
	// PrimaryInterface is the interface that has Primary4 (or Primary6).
	PrimaryInterface string
	Gateways         []netip.Addr
	DNS              []netip.Addr
	// Public is set only when Options.Public is.
	Public []netip.Addr
	// Warnings name the facts that couldn't be read.
	Warnings []string
}

// Options configure Collect.
type Options struct {
	// Public also asks ipify for the public addresses, with Get (default
	// HTTPGet). Off by default: it is the only request to the internet.
	Public bool
	Get    Getter
}

// Collect gathers this machine's network identity. Only the interface list is
// required; other facts that can't be read become warnings.
func Collect(ctx context.Context, src Source, opts Options) (Identity, error) {
	ifaces, err := src.Interfaces()
	if err != nil {
		return Identity{}, err
	}
	id := Identity{Interfaces: ifaces}
	id.Primary4, _ = src.Primary("udp4") // no route: zero, which means "none"
	id.Primary6, _ = src.Primary("udp6")
	id.PrimaryInterface = cmp.Or(interfaceOf(ifaces, id.Primary4), interfaceOf(ifaces, id.Primary6))
	warn := func(what string, err error) {
		if err != nil {
			id.Warnings = append(id.Warnings, what+": "+err.Error())
		}
	}
	id.Gateways, err = src.Gateways(ctx)
	warn("gateway", err)
	id.DNS, err = src.DNSServers(ctx)
	warn("DNS servers", err)
	if opts.Public {
		get := opts.Get
		if get == nil {
			get = HTTPGet
		}
		id.Public, err = PublicIPs(ctx, get)
		warn("public address", err)
	}
	return id, nil
}

// Device is a device on a local network.
type Device struct {
	IP        netip.Addr
	MAC       string
	Vendor    string // see Vendor
	Name      string // from reverse DNS, if any
	Interface string
	Self      bool // this machine
	Gateway   bool // a default gateway
}

// NameResolver looks up the names of addresses; *net.Resolver implements it.
type NameResolver interface {
	LookupAddr(ctx context.Context, addr string) ([]string, error)
}

// DeviceOptions configure Devices.
type DeviceOptions struct {
	// Names, if set, looks up each device's name (reverse DNS), at most
	// NameTimeout (default 1s) each.
	Names       NameResolver
	NameTimeout time.Duration

	emptyHint string // replaces this OS's emptyNeighborHint, for tests
}

// Devices lists the devices in the neighbour cache, plus this machine, with
// vendor hints and names. A neighbour cache that can't be read is a warning.
func Devices(ctx context.Context, src Source, opts DeviceOptions) ([]Device, []string, error) {
	ifaces, err := src.Interfaces()
	if err != nil {
		return nil, nil, err
	}
	var warnings []string
	neighbors, err := src.Neighbors(ctx)
	switch {
	case err != nil:
		warnings = append(warnings, "neighbour cache: "+err.Error())
	case len(neighbors) == 0 && cmp.Or(opts.emptyHint, emptyNeighborHint) != "":
		warnings = append(warnings, "the neighbour cache is empty: "+cmp.Or(opts.emptyHint, emptyNeighborHint))
	}
	gateways, _ := src.Gateways(ctx) // only used to mark the gateway
	primary, _ := src.Primary("udp4")

	byIP := map[netip.Addr]*Device{}
	seenIface := map[string]bool{interfaceOf(ifaces, primary): true}
	for _, n := range neighbors {
		if broadcastOrMulticastMAC(n.MAC) || !n.IP.IsGlobalUnicast() && !n.IP.IsLinkLocalUnicast() {
			continue
		}
		iface := n.Interface
		if a, ok := parseAddr(iface); ok { // Windows names interfaces by address
			iface = cmp.Or(interfaceOf(ifaces, a), iface)
		}
		seenIface[iface] = true
		byIP[n.IP] = &Device{IP: n.IP, MAC: n.MAC, Vendor: Vendor(n.MAC), Interface: iface}
	}
	// This machine, on the interfaces that have neighbours.
	for _, i := range ifaces {
		if i.Kind == KindLoopback || !seenIface[i.Name] {
			continue
		}
		for _, p := range i.Prefixes {
			if p.Addr().Is4() {
				byIP[p.Addr()] = &Device{IP: p.Addr(), MAC: i.MAC, Vendor: Vendor(i.MAC), Interface: i.Name, Self: true}
			}
		}
	}
	out := make([]Device, 0, len(byIP))
	for _, d := range byIP {
		d.Gateway = slices.Contains(gateways, d.IP)
		out = append(out, *d)
	}
	slices.SortFunc(out, func(a, b Device) int { return a.IP.Compare(b.IP) })
	if opts.Names != nil {
		resolveNames(ctx, opts.Names, cmp.Or(opts.NameTimeout, time.Second), out)
	}
	return out, warnings, nil
}

// resolveNames fills in Name by reverse DNS, a few lookups at a time.
func resolveNames(ctx context.Context, r NameResolver, timeout time.Duration, devs []Device) {
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := range devs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			lctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			if names, err := r.LookupAddr(lctx, devs[i].IP.String()); err == nil && len(names) > 0 {
				devs[i].Name = strings.TrimSuffix(names[0], ".")
			}
		})
	}
	wg.Wait()
}

// ProbePorts are the TCP ports `net devices --probe` tries on each host:
// common services of computers, phones, printers and media devices. A
// connection attempt makes the host answer ARP, so it joins the neighbour
// cache even when every port is closed.
const ProbePorts = "22,80,443,445,548,3389,8008,8009,9100,62078"

// DefaultProbeHosts is the largest network probed without raising the limit:
// a /22.
const DefaultProbeHosts = 1024

// ErrNotLocal and ErrTooLarge are why a network can't be probed.
var (
	ErrNotLocal = errors.New("only private and link-local networks can be probed")
	ErrTooLarge = errors.New("network is larger than the probe limit")
)

// ProbeTargets returns the host addresses of p to probe. It refuses networks
// that aren't private (RFC 1918) or link-local IPv4, and networks with more
// than maxHosts hosts.
func ProbeTargets(p netip.Prefix, maxHosts int) ([]netip.Addr, error) {
	p = p.Masked()
	a := p.Addr()
	if !a.Is4() {
		return nil, fmt.Errorf("%s: probing IPv6 networks isn't supported: %w", p, ErrNotLocal)
	}
	if c := Classify(a); c != ClassPrivate && c != ClassLinkLocal {
		return nil, fmt.Errorf("%s is %s: %w", p, c, ErrNotLocal)
	}
	size := 1 << (32 - p.Bits())
	hosts := size
	if size > 2 {
		hosts -= 2 // network and broadcast addresses
	}
	if hosts > maxHosts {
		return nil, fmt.Errorf("%s has %d hosts, more than %d (raise it with --max-hosts): %w", p, hosts, maxHosts, ErrTooLarge)
	}
	out := make([]netip.Addr, 0, hosts)
	for i, cur := 0, a; i < size; i, cur = i+1, cur.Next() {
		if size > 2 && (i == 0 || i == size-1) {
			continue
		}
		out = append(out, cur)
	}
	return out, nil
}

// PrimaryPrefix returns the subnet of the interface address primary.
func PrimaryPrefix(ifaces []Interface, primary netip.Addr) (netip.Prefix, bool) {
	for _, i := range ifaces {
		for _, p := range i.Prefixes {
			if p.Addr() == primary {
				return p.Masked(), true
			}
		}
	}
	return netip.Prefix{}, false
}
