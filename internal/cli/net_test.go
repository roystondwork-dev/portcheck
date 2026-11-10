package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/netinfo"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
	"github.com/roystondwork-dev/portcheck/pkg/scan"
)

// fakeNet is a scripted netinfo.Source.
type fakeNet struct {
	ifaces    []netinfo.Interface
	ifaceErr  error
	primary   netip.Addr
	gateways  []netip.Addr
	dns       []netip.Addr
	dnsErr    error
	neighbors []netinfo.Neighbor
}

func (f *fakeNet) Interfaces() ([]netinfo.Interface, error) { return f.ifaces, f.ifaceErr }
func (f *fakeNet) Primary(network string) (netip.Addr, error) {
	if network == "udp4" && f.primary.IsValid() {
		return f.primary, nil
	}
	return netip.Addr{}, errors.New("no route")
}
func (f *fakeNet) Gateways(context.Context) ([]netip.Addr, error)   { return f.gateways, nil }
func (f *fakeNet) DNSServers(context.Context) ([]netip.Addr, error) { return f.dns, f.dnsErr }
func (f *fakeNet) Neighbors(context.Context) ([]netinfo.Neighbor, error) {
	return f.neighbors, nil
}

// fakeNetDNS answers reverse lookups from a table and records them.
type fakeNetDNS struct {
	mu      sync.Mutex
	ptr     map[string]string
	ips     map[string]string
	lookups []string
}

var errNXDomain = &net.DNSError{Err: "no such host", IsNotFound: true}

func (f *fakeNetDNS) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if v, ok := f.ips[host]; ok {
		return []net.IPAddr{{IP: net.ParseIP(v)}}, nil
	}
	return nil, errNXDomain
}
func (f *fakeNetDNS) LookupCNAME(context.Context, string) (string, error) { return "", errNXDomain }
func (f *fakeNetDNS) LookupMX(context.Context, string) ([]*net.MX, error) { return nil, errNXDomain }
func (f *fakeNetDNS) LookupTXT(context.Context, string) ([]string, error) { return nil, errNXDomain }
func (f *fakeNetDNS) LookupNS(context.Context, string) ([]*net.NS, error) { return nil, errNXDomain }
func (f *fakeNetDNS) LookupAddr(_ context.Context, addr string) ([]string, error) {
	f.mu.Lock()
	f.lookups = append(f.lookups, addr)
	f.mu.Unlock()
	if v, ok := f.ptr[addr]; ok {
		return []string{v}, nil
	}
	return nil, errNXDomain
}

func netHarness() (*harness, *fakeNet, *fakeNetDNS) {
	h := newHarness()
	src := &fakeNet{
		ifaces: []netinfo.Interface{
			{Name: "lo0", Kind: netinfo.KindLoopback, Prefixes: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/8")}},
			{Name: "en0", MAC: "00:00:5e:00:53:0a", MTU: 1500, Kind: netinfo.KindPhysical, Prefixes: []netip.Prefix{netip.MustParsePrefix("192.168.1.10/24")}},
		},
		primary:   netip.MustParseAddr("192.168.1.10"),
		gateways:  []netip.Addr{netip.MustParseAddr("192.168.1.1")},
		dns:       []netip.Addr{netip.MustParseAddr("192.168.1.1")},
		neighbors: []netinfo.Neighbor{{IP: netip.MustParseAddr("192.168.1.1"), MAC: "00:00:5e:00:53:01", Interface: "en0"}},
	}
	dns := &fakeNetDNS{ptr: map[string]string{"192.168.1.1": "router.lan."}, ips: map[string]string{"example.com": "192.0.2.80"}}
	h.app.Net, h.app.DNS = src, dns
	h.app.NetGet = func(context.Context, string) (string, error) { return "203.0.113.7", nil }
	return h, src, dns
}

func TestNetIP(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"net"}, {"net", "ip"}} {
		h, _, _ := netHarness()
		if code := h.run(t, args...); code != ExitOK {
			t.Fatalf("%v: exit %d: %s", args, code, h.stderr)
		}
		for _, want := range []string{"en0        192.168.1.10/24  private   00:00:5e:00:53:0a  primary", "gateway    192.168.1.1", "dns        192.168.1.1"} {
			if !strings.Contains(h.stdout.String(), want) {
				t.Errorf("%v: stdout lacks %q:\n%s", args, want, h.stdout)
			}
		}
		if strings.Contains(h.stdout.String(), "public") {
			t.Errorf("public address shown without --public:\n%s", h.stdout)
		}
	}

	h, src, _ := netHarness()
	src.dnsErr = errors.New("unreadable")
	if code := h.run(t, "net", "--public", "--json"); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	var got struct {
		Data output.NetJSON `json:"data"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Data.Public) != 2 || got.Data.Primary.Interface != "en0" {
		t.Errorf("json = %+v", got.Data)
	}
	if !strings.Contains(h.stderr.String(), "portcheck: DNS servers: unreadable") {
		t.Errorf("stderr lacks the warning:\n%s", h.stderr)
	}

	h, src, _ = netHarness()
	src.ifaceErr = errors.New("no interfaces")
	if code := h.run(t, "net"); code != ExitRuntime {
		t.Errorf("exit %d, want runtime", code)
	}
}

func probeScan(open ...string) ScanFunc {
	return func(_ context.Context, targets []scan.Target, _ portspec.Spec, _ scan.Options) <-chan scan.Result {
		ch := make(chan scan.Result, 2*len(targets))
		for _, tg := range targets {
			state := scan.Closed
			for _, o := range open {
				if tg.Addr.String() == o {
					state = scan.Open
				}
			}
			ch <- scan.Result{Target: tg, Port: 80, State: state}
			if state == scan.Open { // a second open port on the same host
				ch <- scan.Result{Target: tg, Port: 443, State: state}
			}
		}
		close(ch)
		return ch
	}
}

func TestNetDevices(t *testing.T) {
	t.Parallel()
	h, _, dns := netHarness()
	if code := h.run(t, "net", "devices"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	for _, want := range []string{"192.168.1.1   00:00:5e:00:53:01  -       router.lan  en0        gateway", "192.168.1.10  00:00:5e:00:53:0a", "this machine"} {
		if !strings.Contains(h.stdout.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, h.stdout)
		}
	}
	if len(dns.lookups) != 2 {
		t.Errorf("reverse lookups = %v", dns.lookups)
	}

	h, _, dns = netHarness()
	h.run(t, "net", "devices", "--no-names")
	if len(dns.lookups) != 0 || strings.Contains(h.stdout.String(), "router.lan") {
		t.Errorf("--no-names looked up %v", dns.lookups)
	}

	// --probe: a host that answered but isn't in the cache is added.
	h, _, _ = netHarness()
	h.app.Scan = probeScan("192.168.1.50")
	if code := h.run(t, "net", "devices", "--probe", "--json", "--no-names"); code != ExitOK {
		t.Fatalf("probe exit %d: %s", code, h.stderr)
	}
	var got struct {
		Data output.DevicesJSON `json:"data"`
	}
	if err := json.Unmarshal(h.stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Data.Probed != "192.168.1.0/24" || len(got.Data.Devices) != 3 || got.Data.Devices[2].IP != "192.168.1.50" {
		t.Errorf("probe json = %+v", got.Data)
	}
	if !strings.Contains(h.stderr.String(), "probing 254 hosts on 192.168.1.0/24") {
		t.Errorf("no probe notice:\n%s", h.stderr)
	}
}

func TestNetDevicesErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		args  []string
		setup func(*fakeNet)
		code  int
		err   string
	}{
		{"public network", []string{"--probe"}, func(f *fakeNet) {
			f.ifaces[1].Prefixes, f.primary = []netip.Prefix{netip.MustParsePrefix("8.8.8.10/24")}, netip.MustParseAddr("8.8.8.10")
		}, ExitUsage, "only private and link-local networks can be probed"},
		{"too large", []string{"--probe", "--max-hosts", "100"}, nil, ExitUsage, "more than 100"},
		{"no IPv4 route", []string{"--probe"}, func(f *fakeNet) { f.primary = netip.Addr{} }, ExitRuntime, "no IPv4 network to probe"},
		{"interfaces fail (probe)", []string{"--probe"}, func(f *fakeNet) { f.ifaceErr = errors.New("nope") }, ExitRuntime, "nope"},
		{"interfaces fail", nil, func(f *fakeNet) { f.ifaceErr = errors.New("nope") }, ExitRuntime, "nope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, src, _ := netHarness()
			h.app.Scan = probeScan()
			if tc.setup != nil {
				tc.setup(src)
			}
			if code := h.run(t, append([]string{"net", "devices"}, tc.args...)...); code != tc.code {
				t.Errorf("exit %d, want %d: %s", code, tc.code, h.stderr)
			}
			if !strings.Contains(h.stderr.String(), tc.err) {
				t.Errorf("stderr lacks %q:\n%s", tc.err, h.stderr)
			}
		})
	}

	// Interrupted while probing.
	h, _, _ := netHarness()
	h.app.Scan = probeScan()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if code := Execute(ctx, h.app, []string{"net", "devices", "--probe"}); code == ExitOK {
		t.Errorf("cancelled probe succeeded: %s", h.stdout)
	}
}

func TestNetLookup(t *testing.T) {
	t.Parallel()
	h, _, _ := netHarness()
	if code := h.run(t, "net", "lookup", "example.com", "192.168.1.1"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	for _, want := range []string{"example.com  A      192.0.2.80", "192.168.1.1  PTR    router.lan", "192.168.1.1  LOCAL  on a local network", "192.168.1.1  MAC    00:00:5e:00:53:01"} {
		if !strings.Contains(h.stdout.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, h.stdout)
		}
	}
	h, _, _ = netHarness()
	if code := h.run(t, "net", "lookup", "nope.invalid", "example.com"); code != ExitNegative {
		t.Errorf("exit %d, want negative", code)
	}
	if !strings.Contains(h.stderr.String(), "nope.invalid doesn't resolve") || !strings.Contains(h.stdout.String(), "example.com") {
		t.Errorf("stdout:\n%s\nstderr:\n%s", h.stdout, h.stderr)
	}
	h, _, _ = netHarness()
	h.app.Stdout = failWriter{}
	if code := h.run(t, "net", "lookup", "example.com"); code != ExitRuntime {
		t.Errorf("print error exit %d", code)
	}
	h, _, _ = netHarness()
	if code := h.run(t, "net", "lookup"); code != ExitUsage {
		t.Errorf("no args exit %d", code)
	}
}
