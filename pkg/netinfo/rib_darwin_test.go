package netinfo

import (
	"errors"
	"net"
	"net/netip"
	"syscall"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/net/route"
)

func TestNeighborsFromRIB(t *testing.T) {
	// Not parallel: it swaps ifaceByIdx.
	defer func(f func(int) (*net.Interface, error)) { ifaceByIdx = f }(ifaceByIdx)
	ifaceByIdx = func(i int) (*net.Interface, error) {
		if i == 7 {
			return &net.Interface{Name: "en7"}, nil
		}
		return nil, errors.New("no such interface")
	}
	mac := []byte{0x00, 0x00, 0x5e, 0x00, 0x53, 0x01}
	msg := func(index int, addrs ...route.Addr) route.Message {
		return &route.RouteMessage{Index: index, Addrs: addrs}
	}
	msgs := []route.Message{
		msg(4, &route.Inet4Addr{IP: [4]byte{192, 0, 2, 1}}, &route.LinkAddr{Index: 4, Name: "en0", Addr: mac}),
		msg(7, &route.Inet4Addr{IP: [4]byte{192, 0, 2, 7}}, &route.LinkAddr{Index: 7, Addr: mac}),   // name from the index
		msg(9, &route.Inet4Addr{IP: [4]byte{192, 0, 2, 9}}, &route.LinkAddr{Index: 9, Addr: mac}),   // unknown index
		msg(4, &route.Inet4Addr{IP: [4]byte{192, 0, 2, 5}}, &route.LinkAddr{Index: 4, Name: "en0"}), // incomplete
		msg(4, &route.Inet6Addr{}, &route.LinkAddr{Addr: mac}),                                      // not IPv4
		msg(4, &route.Inet4Addr{IP: [4]byte{192, 0, 2, 6}}, &route.Inet4Addr{}),                     // gateway isn't a link
		msg(4, &route.Inet4Addr{}), // too few addresses
		&route.InterfaceMessage{},  // not a route
	}
	want := []Neighbor{
		{IP: netip.MustParseAddr("192.0.2.1"), MAC: "00:00:5e:00:53:01", Interface: "en0"},
		{IP: netip.MustParseAddr("192.0.2.7"), MAC: "00:00:5e:00:53:01", Interface: "en7"},
		{IP: netip.MustParseAddr("192.0.2.9"), MAC: "00:00:5e:00:53:01"},
	}
	if d := cmp.Diff(want, neighborsFromRIB(msgs), cmpAddr); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
}

func TestRIBNeighbors(t *testing.T) {
	// Not parallel: it swaps fetchRIB and parseRIB.
	defer func(f func(int, route.RIBType, int) ([]byte, error), p func(route.RIBType, []byte) ([]route.Message, error)) {
		fetchRIB, parseRIB = f, p
	}(fetchRIB, parseRIB)

	// The real routing socket answers (the table may be empty or hidden).
	if _, err := ribNeighbors(t.Context()); err != nil {
		t.Errorf("real ARP table: %v", err)
	}
	fetchRIB = func(int, route.RIBType, int) ([]byte, error) { return nil, syscall.EPERM }
	if _, err := ribNeighbors(t.Context()); !errors.Is(err, syscall.EPERM) {
		t.Errorf("fetch error = %v", err)
	}
	fetchRIB = func(int, route.RIBType, int) ([]byte, error) { return []byte{1, 2, 3}, nil }
	parseRIB = func(route.RIBType, []byte) ([]route.Message, error) { return nil, errBoom }
	if _, err := ribNeighbors(t.Context()); !errors.Is(err, errBoom) {
		t.Errorf("parse error = %v", err)
	}
}
