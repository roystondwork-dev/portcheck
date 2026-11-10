package netinfo

import (
	"net"
	"net/netip"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestClassify(t *testing.T) {
	t.Parallel()
	for s, want := range map[string]string{
		"127.0.0.1": ClassLoopback, "::1": ClassLoopback,
		"169.254.1.1": ClassLinkLocal, "fe80::1": ClassLinkLocal,
		"10.1.2.3": ClassPrivate, "172.16.0.1": ClassPrivate, "192.168.1.1": ClassPrivate, "fd00::1": ClassPrivate,
		"100.64.0.1":  ClassShared,
		"224.0.0.251": ClassMulticast, "ff02::1": ClassMulticast,
		"0.0.0.0": ClassUnspecified, "::": ClassUnspecified,
		"1.1.1.1": ClassPublic, "2606:4700:4700::1111": ClassPublic,
		"::ffff:192.168.1.1": ClassPrivate,
	} {
		if got := Classify(netip.MustParseAddr(s)); got != want {
			t.Errorf("Classify(%s) = %s, want %s", s, got, want)
		}
	}
	if Classify(netip.Addr{}) != ClassUnspecified {
		t.Error("zero address")
	}
	if Version(netip.MustParseAddr("::ffff:1.2.3.4")) != "IPv4" || Version(netip.MustParseAddr("::1")) != "IPv6" {
		t.Error("Version")
	}
}

func TestInterfaceKind(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"en0": KindPhysical, "eth0": KindPhysical, "Wi-Fi": KindPhysical,
		"utun3": KindVPN, "wg0": KindVPN, "tailscale0": KindVPN, "tun0": KindVPN, "ppp0": KindVPN,
		"docker0": KindVirtual, "br-1a2b": KindVirtual, "veth12": KindVirtual, "awdl0": KindVirtual, "vEthernet (WSL)": KindVirtual,
	} {
		if got := InterfaceKind(name, false); got != want {
			t.Errorf("InterfaceKind(%q) = %s, want %s", name, got, want)
		}
	}
	if InterfaceKind("lo0", true) != KindLoopback {
		t.Error("loopback")
	}
}

func TestMACs(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"AA:BB:CC:DD:EE:FF": "aa:bb:cc:dd:ee:ff",
		"1:0:5e:0:0:fb":     "01:00:5e:00:00:fb",
		"aa-bb-cc-dd-ee-ff": "aa:bb:cc:dd:ee:ff",
		"(incomplete)":      "",
		"aa:bb:cc:dd:ee":    "",
		"aa:bb:cc:dd:ee:gg": "",
		"aaa:bb:cc:dd:ee:f": "",
		"":                  "",
	} {
		if got := NormalizeMAC(in); got != want {
			t.Errorf("NormalizeMAC(%q) = %q, want %q", in, got, want)
		}
	}
	for mac, want := range map[string]bool{"02:00:00:00:00:01": true, "a6:00:00:00:00:01": true, "aa:00:00:00:00:01": true, "00:00:00:00:00:01": false, "b8:27:eb:00:00:01": false, "": false} {
		if got := RandomizedMAC(mac); got != want {
			t.Errorf("RandomizedMAC(%q) = %v", mac, got)
		}
	}
	for mac, want := range map[string]string{
		"b8:27:eb:12:34:56": "Raspberry Pi",
		"02:42:ac:11:00:02": "Docker",
		"da:00:00:00:00:01": "private (randomized) MAC",
		"00:11:22:33:44:55": "",
		"":                  "",
	} {
		if got := Vendor(mac); got != want {
			t.Errorf("Vendor(%q) = %q, want %q", mac, got, want)
		}
	}
	for mac, want := range map[string]bool{"ff:ff:ff:ff:ff:ff": true, "01:00:5e:00:00:fb": true, "33:33:00:00:00:01": true, "aa:bb:cc:00:00:01": false} {
		if got := broadcastOrMulticastMAC(mac); got != want {
			t.Errorf("broadcastOrMulticastMAC(%q) = %v", mac, got)
		}
	}
}

func TestFromNet(t *testing.T) {
	t.Parallel()
	hw, _ := net.ParseMAC("AA:BB:CC:00:00:01")
	ni := net.Interface{Name: "en0", MTU: 1500, HardwareAddr: hw, Flags: net.FlagUp}
	got := fromNet(ni, []net.Addr{
		&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
		&net.IPNet{IP: net.ParseIP("192.0.2.10"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("192.0.2.9"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("2001:db8::5"), Mask: net.CIDRMask(64, 128)},
		&net.IPAddr{IP: net.ParseIP("192.0.2.11")}, // not an IPNet: skipped
	})
	want := Interface{Name: "en0", MAC: "aa:bb:cc:00:00:01", MTU: 1500, Kind: KindPhysical, Prefixes: []netip.Prefix{
		netip.MustParsePrefix("192.0.2.9/24"), netip.MustParsePrefix("192.0.2.10/24"),
		netip.MustParsePrefix("2001:db8::5/64"), netip.MustParsePrefix("fe80::1/64"),
	}}
	if d := cmp.Diff(want, got, cmp.Comparer(func(a, b netip.Prefix) bool { return a == b })); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
	lo := fromNet(net.Interface{Name: "lo0", Flags: net.FlagUp | net.FlagLoopback}, nil)
	if lo.Kind != KindLoopback || lo.MAC != "" {
		t.Errorf("loopback = %+v", lo)
	}
}
