package netinfo

import "net/netip"

// Windows: route.exe and arp.exe. DNS servers aren't read on Windows.
var (
	gatewaySources  = []source[netip.Addr]{{cmd: []string{"route", "print"}, parse: ParseWindowsRoutePrint}}
	dnsSources      []source[netip.Addr]
	neighborSources = []source[Neighbor]{{cmd: []string{"arp", "-a"}, parse: ParseWindowsARP}}
)

// emptyNeighborHint explains an empty neighbour cache, where the OS can hide it.
const emptyNeighborHint = ""
