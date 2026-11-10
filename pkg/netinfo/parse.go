package netinfo

import (
	"encoding/hex"
	"net/netip"
	"strings"
)

// Neighbor is an entry of the OS neighbour (ARP) cache: a device on a local
// network that this machine has talked to recently.
type Neighbor struct {
	IP  netip.Addr
	MAC string // normalised
	// Interface is the interface name, or on Windows the local address of the
	// interface (resolved to a name by Devices).
	Interface string
}

// parseAddr parses an address, dropping a zone ("fe80::1%en0").
func parseAddr(s string) (netip.Addr, bool) {
	s, _, _ = strings.Cut(s, "%")
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

// ParseProcRoute returns the IPv4 default gateways in Linux's /proc/net/route.
func ParseProcRoute(text string) []netip.Addr {
	var out []netip.Addr
	for _, line := range strings.Split(text, "\n")[1:] {
		f := strings.Fields(line)
		// Iface Destination Gateway Flags RefCnt Use Metric Mask ...
		if len(f) < 8 || f[1] != "00000000" || f[7] != "00000000" {
			continue
		}
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 4 {
			continue
		}
		// The kernel prints the address in host (little-endian) byte order.
		a := netip.AddrFrom4([4]byte{b[3], b[2], b[1], b[0]})
		if !a.IsUnspecified() {
			out = append(out, a)
		}
	}
	return out
}

// ParseProcIPv6Route returns the IPv6 default gateways in Linux's
// /proc/net/ipv6_route.
func ParseProcIPv6Route(text string) []netip.Addr {
	var out []netip.Addr
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		// dest destlen src srclen gateway metric refcnt use flags iface
		if len(f) < 10 || f[0] != strings.Repeat("0", 32) || f[1] != "00" {
			continue
		}
		b, err := hex.DecodeString(f[4])
		if err != nil || len(b) != 16 {
			continue
		}
		a := netip.AddrFrom16([16]byte(b))
		if !a.IsUnspecified() {
			out = append(out, a)
		}
	}
	return out
}

// ParseProcARP returns the complete entries of Linux's /proc/net/arp.
func ParseProcARP(text string) []Neighbor {
	var out []Neighbor
	for _, line := range strings.Split(text, "\n")[1:] {
		f := strings.Fields(line)
		// IP address, HW type, Flags, HW address, Mask, Device; flags 0x0 is incomplete.
		if len(f) < 6 || f[2] == "0x0" {
			continue
		}
		ip, ok := parseAddr(f[0])
		mac := NormalizeMAC(f[3])
		if ok && mac != "" && mac != "00:00:00:00:00:00" {
			out = append(out, Neighbor{IP: ip, MAC: mac, Interface: f[5]})
		}
	}
	return out
}

// ParseRouteGet returns the gateway in the output of macOS's
// `route -n get default` (or `route -n get -inet6 default`).
func ParseRouteGet(text string) []netip.Addr {
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || k != "gateway" {
			continue
		}
		if a, ok := parseAddr(strings.TrimSpace(v)); ok {
			return []netip.Addr{a}
		}
	}
	return nil
}

// ParseARPAn returns the entries of macOS's (and the BSDs') `arp -an`, e.g.
// "? (192.0.2.1) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]".
func ParseARPAn(text string) []Neighbor {
	var out []Neighbor
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 6 || f[2] != "at" || f[4] != "on" {
			continue
		}
		ip, ok := parseAddr(strings.Trim(f[1], "()"))
		mac := NormalizeMAC(f[3]) // "(incomplete)" isn't a MAC
		if ok && mac != "" {
			out = append(out, Neighbor{IP: ip, MAC: mac, Interface: f[5]})
		}
	}
	return out
}

// ParseWindowsARP returns the entries of Windows's `arp -a`. Interface is the
// interface's local address, from its "Interface: 192.0.2.10 --- 0xb" header.
func ParseWindowsARP(text string) []Neighbor {
	var out []Neighbor
	iface := ""
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && strings.EqualFold(f[0], "Interface:") {
			iface = f[1]
			continue
		}
		if len(f) != 3 {
			continue
		}
		ip, ok := parseAddr(f[0])
		mac := NormalizeMAC(f[1])
		if ok && mac != "" {
			out = append(out, Neighbor{IP: ip, MAC: mac, Interface: iface})
		}
	}
	return out
}

// ParseWindowsRoutePrint returns the default gateways in Windows's
// `route print`: IPv4 rows "0.0.0.0 0.0.0.0 <gateway> <interface> <metric>"
// and IPv6 rows "<if> <metric> ::/0 <gateway>".
func ParseWindowsRoutePrint(text string) []netip.Addr {
	var out []netip.Addr
	add := func(s string) {
		if a, ok := parseAddr(s); ok && !a.IsUnspecified() {
			for _, have := range out {
				if have == a {
					return
				}
			}
			out = append(out, a)
		}
	}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		switch {
		case len(f) >= 3 && f[0] == "0.0.0.0" && f[1] == "0.0.0.0":
			add(f[2]) // "On-link" doesn't parse
		case len(f) == 4 && f[2] == "::/0":
			add(f[3])
		}
	}
	return out
}

// ParseResolvConf returns the nameservers in a resolv.conf.
func ParseResolvConf(text string) []netip.Addr {
	var out []netip.Addr
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "nameserver" {
			if a, ok := parseAddr(f[1]); ok {
				out = append(out, a)
			}
		}
	}
	return out
}
