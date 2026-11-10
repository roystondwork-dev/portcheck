//go:build !linux && !darwin && !windows

package netinfo

import "net/netip"

// Other systems: interfaces and the primary address only.
var (
	gatewaySources  []source[netip.Addr]
	dnsSources      []source[netip.Addr]
	neighborSources []source[Neighbor]
)

// emptyNeighborHint explains an empty neighbour cache, where the OS can hide it.
const emptyNeighborHint = ""
