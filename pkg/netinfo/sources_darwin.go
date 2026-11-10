package netinfo

import "net/netip"

// macOS: the default routes through route(8); the ARP cache straight from the
// kernel's routing socket (rib_darwin.go); DNS from /etc/resolv.conf, which
// macOS keeps in step with the primary resolver configuration (scutil --dns).
var (
	gatewaySources = []source[netip.Addr]{
		{cmd: []string{"route", "-n", "get", "default"}, parse: ParseRouteGet},
		{cmd: []string{"route", "-n", "get", "-inet6", "default"}, parse: ParseRouteGet},
	}
	dnsSources      = []source[netip.Addr]{{file: "/etc/resolv.conf", parse: ParseResolvConf}}
	neighborSources = []source[Neighbor]{{fn: ribNeighbors}}
)

// emptyNeighborHint explains an empty neighbour cache, where the OS can hide it.
const emptyNeighborHint = "macOS can hide it from apps without Local Network access: allow your terminal app in System Settings → Privacy & Security → Local Network, or use --probe"
