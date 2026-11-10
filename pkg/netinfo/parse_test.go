package netinfo

import (
	"net/netip"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var cmpAddr = cmp.Comparer(func(a, b netip.Addr) bool { return a == b })

func addrs(ss ...string) []netip.Addr {
	out := make([]netip.Addr, len(ss))
	for i, s := range ss {
		out[i] = netip.MustParseAddr(s)
	}
	return out
}

func TestParseProcRoute(t *testing.T) {
	t.Parallel()
	text := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	0102000A	0003	0	0	100	00000000	0	0	0
eth0	0002000A	00000000	0001	0	0	100	00FFFFFF	0	0	0
wlan0	00000000	00000000	0001	0	0	600	00000000	0	0	0
bad	00000000	ZZZZ	0003	0	0	100	00000000	0	0	0
short	00000000
`
	if d := cmp.Diff(addrs("10.0.2.1"), ParseProcRoute(text), cmpAddr); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
}

func TestParseProcIPv6Route(t *testing.T) {
	t.Parallel()
	zero := "00000000000000000000000000000000"
	text := zero + " 00 " + zero + " 00 fe800000000000000000000000000001 00000400 00000001 00000000 00000003 eth0\n" +
		"20010db8000000000000000000000000 40 " + zero + " 00 " + zero + " 00000100 00000001 00000000 00000001 eth0\n" +
		zero + " 00 " + zero + " 00 " + zero + " ffffffff 00000001 00000000 00200200 lo\n" +
		zero + " 00 " + zero + " 00 nothex 00000400 00000001 00000000 00000003 eth0\n" +
		"short\n"
	if d := cmp.Diff(addrs("fe80::1"), ParseProcIPv6Route(text), cmpAddr); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
}

func TestParseProcARP(t *testing.T) {
	t.Parallel()
	text := `IP address       HW type     Flags       HW address            Mask     Device
192.0.2.1        0x1         0x2         AA:BB:CC:00:00:01     *        eth0
192.0.2.7        0x1         0x0         00:00:00:00:00:00     *        eth0
192.0.2.8        0x1         0x2         00:00:00:00:00:00     *        eth0
notanip          0x1         0x2         aa:bb:cc:00:00:02     *        eth0
192.0.2.9        0x1         0x2         nomac                 *        eth0
`
	want := []Neighbor{{IP: netip.MustParseAddr("192.0.2.1"), MAC: "aa:bb:cc:00:00:01", Interface: "eth0"}}
	if d := cmp.Diff(want, ParseProcARP(text), cmpAddr); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
}

func TestParseRouteGet(t *testing.T) {
	t.Parallel()
	v4 := "   route to: default\ndestination: default\n       mask: default\n    gateway: 192.0.2.1\n  interface: en0\n"
	v6 := "   route to: ::\n    gateway: fe80::1%en0\n  interface: en0\n"
	if d := cmp.Diff(addrs("192.0.2.1"), ParseRouteGet(v4), cmpAddr); d != "" {
		t.Errorf("v4 (-want +got):\n%s", d)
	}
	if d := cmp.Diff(addrs("fe80::1"), ParseRouteGet(v6), cmpAddr); d != "" {
		t.Errorf("v6 (-want +got):\n%s", d)
	}
	if got := ParseRouteGet("route: writing to routing socket: not in table\n    gateway: link#14\n"); got != nil {
		t.Errorf("no route = %v", got)
	}
}

func TestParseARPAn(t *testing.T) {
	t.Parallel()
	text := `? (192.0.2.1) at aa:bb:cc:0:0:1 on en0 ifscope [ethernet]
? (192.0.2.5) at (incomplete) on en0 ifscope [ethernet]
? (224.0.0.251) at 1:0:5e:0:0:fb on en0 ifscope permanent [ethernet]
router.lan (192.0.2.254) at aa:bb:cc:00:00:fe on en0 ifscope [ethernet]
? (bad) at aa:bb:cc:00:00:03 on en0
nonsense line
`
	want := []Neighbor{
		{IP: netip.MustParseAddr("192.0.2.1"), MAC: "aa:bb:cc:00:00:01", Interface: "en0"},
		{IP: netip.MustParseAddr("224.0.0.251"), MAC: "01:00:5e:00:00:fb", Interface: "en0"},
		{IP: netip.MustParseAddr("192.0.2.254"), MAC: "aa:bb:cc:00:00:fe", Interface: "en0"},
	}
	if d := cmp.Diff(want, ParseARPAn(text), cmpAddr); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
}

func TestParseWindowsARP(t *testing.T) {
	t.Parallel()
	text := "\r\nInterface: 192.0.2.10 --- 0xb\r\n  Internet Address      Physical Address      Type\r\n" +
		"  192.0.2.1             aa-bb-cc-00-00-01     dynamic   \r\n" +
		"  192.0.2.255           ff-ff-ff-ff-ff-ff     static    \r\n" +
		"  notanip               aa-bb-cc-00-00-02     dynamic   \r\n" +
		"\r\nInterface: 198.51.100.4 --- 0x10\r\n" +
		"  198.51.100.1          aa-bb-cc-00-00-03     dynamic   \r\n"
	want := []Neighbor{
		{IP: netip.MustParseAddr("192.0.2.1"), MAC: "aa:bb:cc:00:00:01", Interface: "192.0.2.10"},
		{IP: netip.MustParseAddr("192.0.2.255"), MAC: "ff:ff:ff:ff:ff:ff", Interface: "192.0.2.10"},
		{IP: netip.MustParseAddr("198.51.100.1"), MAC: "aa:bb:cc:00:00:03", Interface: "198.51.100.4"},
	}
	if d := cmp.Diff(want, ParseWindowsARP(text), cmpAddr); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
}

func TestParseWindowsRoutePrint(t *testing.T) {
	t.Parallel()
	text := `IPv4 Route Table
===========================================================================
Active Routes:
Network Destination        Netmask          Gateway       Interface  Metric
          0.0.0.0          0.0.0.0      192.0.2.1     192.0.2.10     25
          0.0.0.0          0.0.0.0         On-link      10.8.0.2     5
        127.0.0.0        255.0.0.0         On-link       127.0.0.1    331
Persistent Routes:
  Network Address          Netmask  Gateway Address  Metric
          0.0.0.0          0.0.0.0      192.0.2.1  Default
IPv6 Route Table
Active Routes:
 If Metric Network Destination      Gateway
 12    281 ::/0                     fe80::1
  1    331 ::1/128                  On-link
 12    281 ::/0                     ::
`
	if d := cmp.Diff(addrs("192.0.2.1", "fe80::1"), ParseWindowsRoutePrint(text), cmpAddr); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
}

func TestParseResolvConf(t *testing.T) {
	t.Parallel()
	text := "# generated\nsearch lan\nnameserver 192.0.2.53\nnameserver fe80::1%en0\nnameserver bogus\nnameserver\n"
	if d := cmp.Diff(addrs("192.0.2.53", "fe80::1"), ParseResolvConf(text), cmpAddr); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
}
