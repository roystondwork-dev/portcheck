package netinfo

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// fakeSource is a scripted Source.
type fakeSource struct {
	ifaces    []Interface
	ifaceErr  error
	primary   map[string]netip.Addr
	gateways  []netip.Addr
	gwErr     error
	dns       []netip.Addr
	dnsErr    error
	neighbors []Neighbor
	neighErr  error
}

func (f *fakeSource) Interfaces() ([]Interface, error) { return f.ifaces, f.ifaceErr }
func (f *fakeSource) Primary(network string) (netip.Addr, error) {
	if a, ok := f.primary[network]; ok {
		return a, nil
	}
	return netip.Addr{}, errBoom
}
func (f *fakeSource) Gateways(context.Context) ([]netip.Addr, error)   { return f.gateways, f.gwErr }
func (f *fakeSource) DNSServers(context.Context) ([]netip.Addr, error) { return f.dns, f.dnsErr }
func (f *fakeSource) Neighbors(context.Context) ([]Neighbor, error)    { return f.neighbors, f.neighErr }

func pfx(s string) netip.Prefix { return netip.MustParsePrefix(s) }
func ip(s string) netip.Addr    { return netip.MustParseAddr(s) }

func home() *fakeSource {
	return &fakeSource{
		ifaces: []Interface{
			{Name: "lo0", Kind: KindLoopback, Prefixes: []netip.Prefix{pfx("127.0.0.1/8")}},
			{Name: "en0", MAC: "00:00:5e:00:53:0a", MTU: 1500, Kind: KindPhysical, Prefixes: []netip.Prefix{pfx("192.0.2.10/24"), pfx("2001:db8::10/64")}},
			{Name: "docker0", Kind: KindVirtual, Prefixes: []netip.Prefix{pfx("172.17.0.1/16")}},
		},
		primary:  map[string]netip.Addr{"udp4": ip("192.0.2.10"), "udp6": ip("2001:db8::10")},
		gateways: []netip.Addr{ip("192.0.2.1")},
		dns:      []netip.Addr{ip("192.0.2.53")},
		neighbors: []Neighbor{
			{IP: ip("192.0.2.1"), MAC: "00:00:5e:00:53:01", Interface: "en0"},
			{IP: ip("192.0.2.20"), MAC: "b8:27:eb:00:00:20", Interface: "en0"},
			{IP: ip("192.0.2.255"), MAC: "ff:ff:ff:ff:ff:ff", Interface: "en0"},
			{IP: ip("224.0.0.251"), MAC: "01:00:5e:00:00:fb", Interface: "en0"},
			{IP: ip("239.255.255.250"), MAC: "00:00:5e:00:53:99", Interface: "en0"},
			{IP: ip("192.0.2.30"), MAC: "da:00:00:00:00:30", Interface: "192.0.2.10"}, // Windows-style
		},
	}
}

func TestCollect(t *testing.T) {
	t.Parallel()
	src := home()
	id, err := Collect(t.Context(), src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Primary4 != ip("192.0.2.10") || id.Primary6 != ip("2001:db8::10") || id.PrimaryInterface != "en0" {
		t.Errorf("primary = %v %v %q", id.Primary4, id.Primary6, id.PrimaryInterface)
	}
	if len(id.Gateways) != 1 || len(id.DNS) != 1 || id.Public != nil || id.Warnings != nil {
		t.Errorf("id = %+v", id)
	}

	src.gwErr, src.dnsErr, src.primary = ErrUnsupported, errBoom, map[string]netip.Addr{"udp6": ip("2001:db8::10")}
	called := ""
	id, _ = Collect(t.Context(), src, Options{Public: true, Get: func(_ context.Context, url string) (string, error) {
		called += url + " "
		return "203.0.113.7", nil
	}})
	if id.PrimaryInterface != "en0" || id.Primary4.IsValid() {
		t.Errorf("IPv6-only primary = %+v", id)
	}
	want := []string{"gateway: " + ErrUnsupported.Error(), "DNS servers: boom"}
	if d := cmp.Diff(want, id.Warnings); d != "" {
		t.Errorf("warnings (-want +got):\n%s", d)
	}
	if len(id.Public) != 2 || !strings.Contains(called, "ipify") {
		t.Errorf("public = %v (called %s)", id.Public, called)
	}
	id, _ = Collect(t.Context(), src, Options{Public: true, Get: func(context.Context, string) (string, error) { return "", errBoom }})
	if len(id.Warnings) != 3 || !strings.HasPrefix(id.Warnings[2], "public address") {
		t.Errorf("warnings = %v", id.Warnings)
	}

	src.ifaceErr = errBoom
	if _, err := Collect(t.Context(), src, Options{}); !errors.Is(err, errBoom) {
		t.Errorf("error = %v", err)
	}
}

func TestCollectDefaultGetter(t *testing.T) {
	t.Parallel()
	// Public with no Getter uses HTTPGet; a cancelled context keeps it offline.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	id, err := Collect(ctx, home(), Options{Public: true})
	if err != nil || len(id.Warnings) != 1 || !strings.HasPrefix(id.Warnings[0], "public address") {
		t.Errorf("id = %+v, %v", id, err)
	}
}

type fakeNames map[string]string

func (f fakeNames) LookupAddr(ctx context.Context, addr string) ([]string, error) {
	if addr == "192.0.2.20" {
		<-ctx.Done() // a slow resolver: the per-lookup timeout gives up
		return nil, ctx.Err()
	}
	if n, ok := f[addr]; ok {
		return []string{n}, nil
	}
	return nil, &net.DNSError{Err: "not found", IsNotFound: true}
}

func TestDevices(t *testing.T) {
	t.Parallel()
	devs, warnings, err := Devices(t.Context(), home(), DeviceOptions{
		Names:       fakeNames{"192.0.2.1": "router.lan.", "192.0.2.10": "laptop.lan."},
		NameTimeout: 20 * time.Millisecond,
	})
	if err != nil || warnings != nil {
		t.Fatalf("Devices: %v %v", warnings, err)
	}
	want := []Device{
		{IP: ip("192.0.2.1"), MAC: "00:00:5e:00:53:01", Interface: "en0", Name: "router.lan", Gateway: true},
		{IP: ip("192.0.2.10"), MAC: "00:00:5e:00:53:0a", Interface: "en0", Name: "laptop.lan", Self: true},
		{IP: ip("192.0.2.20"), MAC: "b8:27:eb:00:00:20", Vendor: "Raspberry Pi", Interface: "en0"},
		{IP: ip("192.0.2.30"), MAC: "da:00:00:00:00:30", Vendor: "private (randomized) MAC", Interface: "en0"},
	}
	if d := cmp.Diff(want, devs, cmpAddr); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}

	// Without names, a cache that can't be read, and no primary route.
	src := home()
	src.neighErr, src.neighbors, src.primary = ErrUnsupported, nil, nil
	devs, warnings, err = Devices(t.Context(), src, DeviceOptions{})
	if err != nil || len(devs) != 0 || len(warnings) != 1 {
		t.Errorf("unsupported = %+v %v %v", devs, warnings, err)
	}
	// An empty cache, on an OS that can hide it, gets a hint.
	src.neighErr = nil
	_, warnings, _ = Devices(t.Context(), src, DeviceOptions{emptyHint: "allow it"})
	if len(warnings) != 1 || warnings[0] != "the neighbour cache is empty: allow it" {
		t.Errorf("hint = %v", warnings)
	}
	src.ifaceErr = errBoom
	if _, _, err := Devices(t.Context(), src, DeviceOptions{}); !errors.Is(err, errBoom) {
		t.Errorf("error = %v", err)
	}
	// An unknown Windows interface address is kept as it is.
	src = home()
	src.neighbors = []Neighbor{{IP: ip("198.51.100.9"), MAC: "00:00:5e:00:53:09", Interface: "198.51.100.4"}}
	devs, _, _ = Devices(t.Context(), src, DeviceOptions{})
	if len(devs) != 2 || devs[1].Interface != "198.51.100.4" {
		t.Errorf("unknown interface = %+v", devs)
	}
}

func TestProbeTargets(t *testing.T) {
	t.Parallel()
	got, err := ProbeTargets(pfx("192.0.2.77/30"), DefaultProbeHosts)
	if err == nil {
		t.Fatalf("documentation range accepted: %v", got)
	}
	got, err = ProbeTargets(pfx("192.168.1.77/30"), DefaultProbeHosts)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(addrs("192.168.1.77", "192.168.1.78"), got, cmpAddr); d != "" {
		t.Errorf("/30 (-want +got):\n%s", d)
	}
	if got, _ := ProbeTargets(pfx("10.0.0.0/24"), DefaultProbeHosts); len(got) != 254 || got[0] != ip("10.0.0.1") || got[253] != ip("10.0.0.254") {
		t.Errorf("/24 = %d hosts", len(got))
	}
	if got, _ := ProbeTargets(pfx("10.0.0.4/31"), DefaultProbeHosts); len(got) != 2 {
		t.Errorf("/31 = %v", got)
	}
	if got, _ := ProbeTargets(pfx("169.254.0.0/22"), DefaultProbeHosts); len(got) != 1022 {
		t.Errorf("link-local /22 = %d", len(got))
	}
	for _, tc := range []struct {
		p    string
		max  int
		want error
	}{
		{"10.0.0.0/16", DefaultProbeHosts, ErrTooLarge},
		{"8.8.8.0/24", DefaultProbeHosts, ErrNotLocal},
		{"100.64.0.0/24", DefaultProbeHosts, ErrNotLocal},
		{"fd00::/120", DefaultProbeHosts, ErrNotLocal},
		{"10.0.0.0/24", 100, ErrTooLarge},
	} {
		if _, err := ProbeTargets(pfx(tc.p), tc.max); !errors.Is(err, tc.want) {
			t.Errorf("ProbeTargets(%s) error = %v, want %v", tc.p, err, tc.want)
		}
	}
	if got, err := ProbeTargets(pfx("10.0.0.0/16"), 70000); err != nil || len(got) != 65534 {
		t.Errorf("raised limit = %d, %v", len(got), err)
	}
}

func TestPrimaryPrefix(t *testing.T) {
	t.Parallel()
	ifaces := home().ifaces
	if p, ok := PrimaryPrefix(ifaces, ip("192.0.2.10")); !ok || p != pfx("192.0.2.0/24") {
		t.Errorf("prefix = %v %v", p, ok)
	}
	if _, ok := PrimaryPrefix(ifaces, ip("198.51.100.1")); ok {
		t.Error("unknown address has a prefix")
	}
}
