package netinfo

import "net/netip"

// Linux: everything from procfs and /etc/resolv.conf.
var (
	gatewaySources = []source[netip.Addr]{
		{file: "/proc/net/route", parse: ParseProcRoute},
		{file: "/proc/net/ipv6_route", parse: ParseProcIPv6Route},
	}
	dnsSources      = []source[netip.Addr]{{file: "/etc/resolv.conf", parse: ParseResolvConf}}
	neighborSources = []source[Neighbor]{{file: "/proc/net/arp", parse: ParseProcARP}}
)

// emptyNeighborHint explains an empty neighbour cache, where the OS can hide it.
const emptyNeighborHint = ""
