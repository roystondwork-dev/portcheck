package netinfo

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"syscall"

	"golang.org/x/net/route"
)

// The routing socket, behind variables for tests.
var (
	fetchRIB    = route.FetchRIB
	parseRIB    = route.ParseRIB
	ifaceByIdx  = net.InterfaceByIndex
	arpRIBType  = route.RIBType(syscall.NET_RT_FLAGS)
	arpRIBFlags = syscall.RTF_LLINFO
)

// ribNeighbors reads the IPv4 ARP table from the kernel's routing socket, the
// way arp(8) does, without running it.
func ribNeighbors(context.Context) ([]Neighbor, error) {
	b, err := fetchRIB(syscall.AF_INET, arpRIBType, arpRIBFlags)
	if err != nil {
		return nil, fmt.Errorf("read the ARP table: %w", err)
	}
	msgs, err := parseRIB(arpRIBType, b)
	if err != nil {
		return nil, fmt.Errorf("read the ARP table: %w", err)
	}
	return neighborsFromRIB(msgs), nil
}

// neighborsFromRIB turns ARP routing messages into neighbours: the
// destination is the IPv4 address, the gateway its link-layer address.
func neighborsFromRIB(msgs []route.Message) []Neighbor {
	var out []Neighbor
	for _, m := range msgs {
		rm, ok := m.(*route.RouteMessage)
		if !ok || len(rm.Addrs) <= syscall.RTAX_GATEWAY {
			continue
		}
		dst, ok := rm.Addrs[syscall.RTAX_DST].(*route.Inet4Addr)
		link, ok2 := rm.Addrs[syscall.RTAX_GATEWAY].(*route.LinkAddr)
		if !ok || !ok2 || len(link.Addr) != 6 {
			continue // an incomplete entry has no link-layer address yet
		}
		name := link.Name
		if name == "" {
			if ni, err := ifaceByIdx(rm.Index); err == nil {
				name = ni.Name
			}
		}
		out = append(out, Neighbor{IP: netip.AddrFrom4(dst.IP), MAC: NormalizeMAC(net.HardwareAddr(link.Addr).String()), Interface: name})
	}
	return out
}
