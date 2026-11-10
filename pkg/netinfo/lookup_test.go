package netinfo

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// fakeDNS answers from tables; a missing entry is "not found".
type fakeDNS struct {
	ips   map[string][]string
	cname map[string]string
	mx    map[string][]*net.MX
	txt   map[string][]string
	ns    map[string][]*net.NS
	ptr   map[string][]string
}

var notFound = &net.DNSError{Err: "no such host", IsNotFound: true}

func (f fakeDNS) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	v, ok := f.ips[host]
	if !ok {
		return nil, notFound
	}
	out := make([]net.IPAddr, len(v))
	for i, s := range v {
		out[i] = net.IPAddr{IP: net.ParseIP(s)}
	}
	return out, nil
}

func (f fakeDNS) LookupCNAME(_ context.Context, host string) (string, error) {
	if v, ok := f.cname[host]; ok {
		return v, nil
	}
	return "", notFound
}

func (f fakeDNS) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	if v, ok := f.mx[name]; ok {
		return v, nil
	}
	return nil, notFound
}

func (f fakeDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	if v, ok := f.txt[name]; ok {
		return v, nil
	}
	return nil, notFound
}

func (f fakeDNS) LookupNS(_ context.Context, name string) ([]*net.NS, error) {
	if v, ok := f.ns[name]; ok {
		return v, nil
	}
	return nil, notFound
}

func (f fakeDNS) LookupAddr(_ context.Context, addr string) ([]string, error) {
	if v, ok := f.ptr[addr]; ok {
		return v, nil
	}
	return nil, notFound
}

var dns = fakeDNS{
	ips: map[string][]string{
		"example.com":     {"2001:db8::1", "192.0.2.80", "192.0.2.80", "198.51.100.80"},
		"www.example.com": {"192.0.2.80"},
		"bad.example":     {"not-an-ip"},
	},
	cname: map[string]string{"example.com": "example.com.", "www.example.com": "example.com."},
	mx:    map[string][]*net.MX{"example.com": {{Host: "mail.example.com.", Pref: 10}}},
	txt:   map[string][]string{"example.com": {"v=spf1 -all"}, "txt-only.example": {"hello"}},
	ns:    map[string][]*net.NS{"example.com": {{Host: "ns1.example.com."}}},
	ptr:   map[string][]string{"192.0.2.20": {"pi.lan."}},
}

func TestLookupDomain(t *testing.T) {
	t.Parallel()
	got := Lookup(t.Context(), dns, Local{}, "example.com.")
	want := LookupResult{Query: "example.com", Kind: "domain", Records: []Record{
		{Type: "A", Value: "192.0.2.80", Note: ClassPublic},
		{Type: "A", Value: "198.51.100.80", Note: ClassPublic},
		{Type: "AAAA", Value: "2001:db8::1", Note: ClassPublic},
		{Type: "MX", Value: "mail.example.com", Note: "preference 10"},
		{Type: "NS", Value: "ns1.example.com"},
		{Type: "TXT", Value: "v=spf1 -all"},
	}}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
	www := Lookup(t.Context(), dns, Local{}, "www.example.com")
	if d := cmp.Diff([]Record{{Type: "A", Value: "192.0.2.80", Note: ClassPublic}, {Type: "CNAME", Value: "example.com"}}, www.Records); d != "" {
		t.Errorf("www (-want +got):\n%s", d)
	}
	if r := Lookup(t.Context(), dns, Local{}, "txt-only.example"); r.Err != "" || len(r.Records) != 1 {
		t.Errorf("TXT only = %+v", r)
	}
	if r := Lookup(t.Context(), dns, Local{}, "bad.example"); r.Err != "" || len(r.Records) != 0 {
		t.Errorf("garbage address = %+v", r)
	}
	missing := Lookup(t.Context(), dns, Local{}, "nope.invalid")
	if !strings.Contains(missing.Err, "nope.invalid doesn't resolve") {
		t.Errorf("missing = %+v", missing)
	}
}

func TestLookupIP(t *testing.T) {
	t.Parallel()
	local := Local{Interfaces: home().ifaces, Neighbors: home().neighbors}
	pi := Lookup(t.Context(), dns, local, "192.0.2.20")
	want := LookupResult{Query: "192.0.2.20", Kind: "ip", Records: []Record{
		{Type: "CLASS", Value: ClassPublic, Note: "IPv4"}, // 192.0.2.0/24 is a documentation range: "public"
		{Type: "PTR", Value: "pi.lan"},
		{Type: "LOCAL", Value: "on a local network", Note: "en0"},
		{Type: "MAC", Value: "b8:27:eb:00:00:20", Note: "Raspberry Pi"},
	}}
	if d := cmp.Diff(want, pi); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
	self := Lookup(t.Context(), dns, local, "192.0.2.10")
	if d := cmp.Diff(Record{Type: "LOCAL", Value: "this machine", Note: "en0"}, self.Records[1]); d != "" {
		t.Errorf("self (-want +got):\n%s", d)
	}
	unknownNeighbor := Lookup(t.Context(), dns, local, "192.0.2.99")
	if len(unknownNeighbor.Records) != 2 {
		t.Errorf("on-link without a cache entry = %+v", unknownNeighbor.Records)
	}
	far := Lookup(t.Context(), dns, local, "::1")
	if d := cmp.Diff([]Record{{Type: "CLASS", Value: ClassLoopback, Note: "IPv6"}}, far.Records); d != "" {
		t.Errorf("loopback is not on-link (-want +got):\n%s", d)
	}
}
