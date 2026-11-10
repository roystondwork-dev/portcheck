// Package netinfo describes this machine on the network: its interfaces and
// addresses, the primary address and default gateway, DNS servers, the devices
// in its neighbour (ARP) cache, and DNS facts about names and addresses.
//
// Everything is read locally (interfaces, routing table, neighbour cache,
// resolver configuration) without root. Two things leave the machine, and only
// when asked: PublicIPs makes an HTTPS request to ipify, and probing (see
// ProbeTargets) is done by the caller with pkg/scan. See ADR-0008.
package netinfo

import (
	"net"
	"net/netip"
	"slices"
	"strings"
)

// Interface is a network interface that is up, with its addresses.
type Interface struct {
	Name     string
	MAC      string // lower-case, colon-separated; "" for interfaces without one
	MTU      int
	Kind     string // see InterfaceKind
	Prefixes []netip.Prefix
}

// Interface kinds.
const (
	KindLoopback = "loopback"
	KindVPN      = "vpn"
	KindVirtual  = "virtual"
	KindPhysical = "physical"
)

// InterfaceKind guesses what an interface is from its name: "loopback", "vpn"
// (utun, tun, tap, wg, tailscale, ppp, ipsec), "virtual" (Docker, VM and Apple
// peer-to-peer bridges) or "physical". It is a hint, not a guarantee.
func InterfaceKind(name string, loopback bool) string {
	if loopback {
		return KindLoopback
	}
	n := strings.ToLower(name)
	for _, p := range []string{"utun", "tun", "tap", "wg", "tailscale", "ppp", "ipsec", "zt"} {
		if strings.HasPrefix(n, p) {
			return KindVPN
		}
	}
	for _, p := range []string{"docker", "br-", "veth", "vmnet", "vboxnet", "virbr", "bridge", "awdl", "llw", "vethernet", "anpi", "ap1", "gif", "stf"} {
		if strings.HasPrefix(n, p) {
			return KindVirtual
		}
	}
	return KindPhysical
}

// Address classes.
const (
	ClassLoopback    = "loopback"
	ClassLinkLocal   = "link-local"
	ClassPrivate     = "private"
	ClassShared      = "shared" // carrier-grade NAT, 100.64.0.0/10
	ClassMulticast   = "multicast"
	ClassUnspecified = "unspecified"
	ClassPublic      = "public"
)

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// Classify says what kind of address a is: loopback, link-local, private
// (RFC 1918 and IPv6 unique local), shared (carrier-grade NAT), multicast,
// unspecified or public.
func Classify(a netip.Addr) string {
	a = a.Unmap()
	switch {
	case !a.IsValid() || a.IsUnspecified():
		return ClassUnspecified
	case a.IsLoopback():
		return ClassLoopback
	case a.IsMulticast():
		return ClassMulticast
	case a.IsLinkLocalUnicast():
		return ClassLinkLocal
	case a.IsPrivate():
		return ClassPrivate
	case cgnat.Contains(a):
		return ClassShared
	default:
		return ClassPublic
	}
}

// Version is "IPv4" or "IPv6".
func Version(a netip.Addr) string {
	if a.Unmap().Is4() {
		return "IPv4"
	}
	return "IPv6"
}

// NormalizeMAC returns mac lower-case with colons and two digits per byte
// ("1:0:5E:0:0:FB" and "01-00-5e-00-00-fb" become "01:00:5e:00:00:fb"), or ""
// if it isn't a MAC address.
func NormalizeMAC(mac string) string {
	parts := strings.FieldsFunc(strings.ToLower(mac), func(r rune) bool { return r == ':' || r == '-' })
	if len(parts) != 6 {
		return ""
	}
	for i, p := range parts {
		if len(p) == 1 {
			p = "0" + p
		}
		if len(p) != 2 || strings.Trim(p, "0123456789abcdef") != "" {
			return ""
		}
		parts[i] = p
	}
	return strings.Join(parts, ":")
}

// RandomizedMAC reports whether mac (normalised) is locally administered, as
// phones and laptops use for privacy ("private Wi-Fi address").
func RandomizedMAC(mac string) bool {
	if len(mac) < 2 {
		return false
	}
	return strings.ContainsRune("2367abef", rune(mac[1]))
}

// broadcastOrMulticastMAC reports MACs that aren't a device: broadcast, and
// IPv4/IPv6 multicast.
func broadcastOrMulticastMAC(mac string) bool {
	return mac == "ff:ff:ff:ff:ff:ff" || strings.HasPrefix(mac, "01:00:5e:") || strings.HasPrefix(mac, "33:33:")
}

// fromNet converts a net.Interface and its addresses.
func fromNet(ni net.Interface, addrs []net.Addr) Interface {
	out := Interface{
		Name: ni.Name,
		MAC:  NormalizeMAC(ni.HardwareAddr.String()),
		MTU:  ni.MTU,
		Kind: InterfaceKind(ni.Name, ni.Flags&net.FlagLoopback != 0),
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			ip, _ := netip.AddrFromSlice(n.IP)
			ones, _ := n.Mask.Size()
			out.Prefixes = append(out.Prefixes, netip.PrefixFrom(ip.Unmap(), ones))
		}
	}
	// By address: IPv4 sorts before IPv6.
	slices.SortStableFunc(out.Prefixes, func(a, b netip.Prefix) int { return a.Addr().Compare(b.Addr()) })
	return out
}

// interfaceOf returns the name of the interface that has address a, or "".
func interfaceOf(ifaces []Interface, a netip.Addr) string {
	for _, i := range ifaces {
		for _, p := range i.Prefixes {
			if p.Addr() == a {
				return i.Name
			}
		}
	}
	return ""
}

// onLink returns the interface whose subnet contains a (not loopback), or "".
func onLink(ifaces []Interface, a netip.Addr) string {
	for _, i := range ifaces {
		if i.Kind == KindLoopback {
			continue
		}
		for _, p := range i.Prefixes {
			if p.Masked().Contains(a) {
				return i.Name
			}
		}
	}
	return ""
}
