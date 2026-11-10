package netinfo

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
)

// DNSResolver is the system resolver; *net.Resolver implements it.
type DNSResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
	LookupCNAME(ctx context.Context, host string) (string, error)
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupNS(ctx context.Context, name string) ([]*net.NS, error)
	LookupAddr(ctx context.Context, addr string) ([]string, error)
}

// Record is one fact about a looked-up name or address.
type Record struct {
	// Type is a DNS record type (A, AAAA, CNAME, MX, NS, TXT, PTR), or for an
	// address CLASS, LOCAL or MAC.
	Type  string
	Value string
	// Note adds detail: an address's class, an MX preference, a MAC's vendor.
	Note string
}

// LookupResult is what Lookup found for one query.
type LookupResult struct {
	Query   string
	Kind    string // "domain" or "ip"
	Records []Record
	// Err is set when a domain doesn't resolve.
	Err string
}

// Local is what Lookup knows about this machine, to say whether an address is
// local and which device it is.
type Local struct {
	Interfaces []Interface
	Neighbors  []Neighbor
}

// Lookup describes a domain name (its A, AAAA, CNAME, MX, NS and TXT records)
// or an IP address (its class, reverse DNS, and whether it is this machine or
// on a local network), using the system resolver only.
func Lookup(ctx context.Context, r DNSResolver, local Local, query string) LookupResult {
	if a, ok := parseAddr(query); ok {
		return lookupIP(ctx, r, local, a)
	}
	return lookupDomain(ctx, r, strings.TrimSuffix(query, "."))
}

func addrRecord(a netip.Addr) Record {
	t := "A"
	if !a.Is4() {
		t = "AAAA"
	}
	return Record{Type: t, Value: a.String(), Note: Classify(a)}
}

func lookupDomain(ctx context.Context, r DNSResolver, name string) LookupResult {
	res := LookupResult{Query: name, Kind: "domain"}
	ips, ipErr := r.LookupIPAddr(ctx, name)
	var addrs []netip.Addr
	for _, ip := range ips {
		if a, ok := netip.AddrFromSlice(ip.IP); ok {
			addrs = append(addrs, a.Unmap())
		}
	}
	slices.SortFunc(addrs, netip.Addr.Compare) // IPv4 sorts before IPv6
	for _, a := range slices.Compact(addrs) {
		res.Records = append(res.Records, addrRecord(a))
	}
	if cname, err := r.LookupCNAME(ctx, name); err == nil {
		if c := strings.TrimSuffix(cname, "."); c != "" && !strings.EqualFold(c, name) {
			res.Records = append(res.Records, Record{Type: "CNAME", Value: c})
		}
	}
	if mxs, err := r.LookupMX(ctx, name); err == nil {
		for _, mx := range mxs {
			res.Records = append(res.Records, Record{Type: "MX", Value: strings.TrimSuffix(mx.Host, "."), Note: fmt.Sprintf("preference %d", mx.Pref)})
		}
	}
	if nss, err := r.LookupNS(ctx, name); err == nil {
		for _, ns := range nss {
			res.Records = append(res.Records, Record{Type: "NS", Value: strings.TrimSuffix(ns.Host, ".")})
		}
	}
	if txts, err := r.LookupTXT(ctx, name); err == nil {
		for _, t := range txts {
			res.Records = append(res.Records, Record{Type: "TXT", Value: t})
		}
	}
	if ipErr != nil && len(res.Records) == 0 {
		res.Err = fmt.Sprintf("%s doesn't resolve: %v", name, ipErr)
	}
	return res
}

func lookupIP(ctx context.Context, r DNSResolver, local Local, a netip.Addr) LookupResult {
	res := LookupResult{Query: a.String(), Kind: "ip"}
	res.Records = append(res.Records, Record{Type: "CLASS", Value: Classify(a), Note: Version(a)})
	if names, err := r.LookupAddr(ctx, a.String()); err == nil {
		for _, n := range names {
			res.Records = append(res.Records, Record{Type: "PTR", Value: strings.TrimSuffix(n, ".")})
		}
	}
	switch iface := interfaceOf(local.Interfaces, a); {
	case iface != "":
		res.Records = append(res.Records, Record{Type: "LOCAL", Value: "this machine", Note: iface})
	case onLink(local.Interfaces, a) != "":
		res.Records = append(res.Records, Record{Type: "LOCAL", Value: "on a local network", Note: onLink(local.Interfaces, a)})
		for _, n := range local.Neighbors {
			if n.IP == a {
				res.Records = append(res.Records, Record{Type: "MAC", Value: n.MAC, Note: Vendor(n.MAC)})
				break
			}
		}
	}
	return res
}
