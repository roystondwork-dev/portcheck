package scan

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

type fakeResolver map[string][]netip.Addr

func (r fakeResolver) LookupNetIP(_ context.Context, network, host string) ([]netip.Addr, error) {
	if network != "ip" {
		return nil, errors.New("unexpected network " + network)
	}
	addrs, ok := r[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	return addrs, nil
}

var resolver = fakeResolver{
	"db.internal":  {netip.MustParseAddr("2001:db8::5"), netip.MustParseAddr("10.0.0.5")},
	"v6.internal":  {netip.MustParseAddr("2001:db8::6")},
	"mapped.local": {netip.MustParseAddr("::ffff:192.0.2.7")},
	"empty.local":  {},
	"dup.local":    {netip.MustParseAddr("10.0.0.1")},
}

func targetsString(ts []Target) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = t.String()
	}
	return strings.Join(parts, " ")
}

func TestExpand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		specs []string
		max   int
		want  string
	}{
		{"ipv4", []string{"192.0.2.1"}, 0, "192.0.2.1"},
		{"ipv6 bracketed", []string{"[2001:db8::1]"}, 0, "2001:db8::1"},
		{"v4-mapped unmapped", []string{"::ffff:192.0.2.9"}, 0, "192.0.2.9"},
		{"cidr", []string{"10.0.0.0/30"}, 0, "10.0.0.0 10.0.0.1 10.0.0.2 10.0.0.3"},
		{"cidr unmasked", []string{"10.0.0.6/31"}, 0, "10.0.0.6 10.0.0.7"},
		{"cidr /32", []string{" 10.0.0.9/32 "}, 0, "10.0.0.9"},
		{"ipv6 cidr", []string{"2001:db8::/127"}, 0, "2001:db8:: 2001:db8::1"},
		{"hostname prefers ipv4", []string{"db.internal"}, 0, "db.internal (10.0.0.5)"},
		{"hostname ipv6 only", []string{"v6.internal"}, 0, "v6.internal (2001:db8::6)"},
		{"hostname mapped", []string{"mapped.local"}, 0, "mapped.local (192.0.2.7)"},
		{"dedupe keeps first", []string{"10.0.0.1", "10.0.0.0/31", "dup.local"}, 0, "10.0.0.1 10.0.0.0"},
		{"exactly at limit", []string{"10.0.0.0/30"}, 4, "10.0.0.0 10.0.0.1 10.0.0.2 10.0.0.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Expand(t.Context(), resolver, tt.specs, tt.max)
			if err != nil {
				t.Fatal(err)
			}
			if s := targetsString(got); s != tt.want {
				t.Errorf("Expand(%q) = %s, want %s", tt.specs, s, tt.want)
			}
		})
	}
}

func TestExpandErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		specs   []string
		max     int
		wantErr error
		msg     string
	}{
		{"empty", []string{""}, 0, ErrBadTarget, ""},
		{"spaces inside", []string{"a b"}, 0, ErrBadTarget, ""},
		{"leading dash", []string{"-evil"}, 0, ErrBadTarget, ""},
		{"leading dot", []string{".x"}, 0, ErrBadTarget, ""},
		{"double dot", []string{"a..b"}, 0, ErrBadTarget, ""},
		{"too long", []string{strings.Repeat("a", 254)}, 0, ErrBadTarget, ""},
		{"url", []string{"http://x"}, 0, ErrBadTarget, ""},
		{"unresolvable", []string{"nope.invalid"}, 0, ErrResolve, "no such host"},
		{"no addresses", []string{"empty.local"}, 0, ErrResolve, "no addresses"},
		{"huge prefix", []string{"10.0.0.0/1"}, 0, ErrTooManyHosts, "2^31 addresses"},
		{"ipv6 /64", []string{"2001:db8::/64"}, 0, ErrTooManyHosts, "2^64"},
		{"prefix above limit", []string{"10.0.0.0/22"}, 512, ErrTooManyHosts, "1024 addresses, limit is 512"},
		{"accumulated over limit", []string{"10.0.0.0/30", "10.0.1.0/30"}, 6, ErrTooManyHosts, "more than 6"},
		{"single hosts over limit", []string{"10.0.0.1", "10.0.0.2"}, 1, ErrTooManyHosts, ""},
		{"hostname over limit", []string{"10.0.0.1", "db.internal"}, 1, ErrTooManyHosts, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Expand(t.Context(), resolver, tt.specs, tt.max)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.msg) {
				t.Errorf("error %q does not mention %q", err, tt.msg)
			}
		})
	}
}

func TestExpandDefaultLimit(t *testing.T) {
	t.Parallel()
	got, err := Expand(t.Context(), resolver, []string{"10.0.0.0/22"}, 0)
	if err != nil || len(got) != DefaultMaxHosts {
		t.Errorf("len = %d, err = %v; want %d hosts", len(got), err, DefaultMaxHosts)
	}
	if _, err := Expand(t.Context(), resolver, []string{"10.0.0.0/21"}, -1); !errors.Is(err, ErrTooManyHosts) {
		t.Errorf("error = %v, want ErrTooManyHosts", err)
	}
}

func TestTargetString(t *testing.T) {
	t.Parallel()
	a := netip.MustParseAddr("10.0.0.5")
	if got := (Target{Host: "db", Addr: a}).String(); got != "db (10.0.0.5)" {
		t.Errorf("String = %q", got)
	}
	if got := (Target{Host: "10.0.0.5", Addr: a}).String(); got != "10.0.0.5" {
		t.Errorf("String = %q", got)
	}
}

func FuzzExpand(f *testing.F) {
	for _, s := range []string{"10.0.0.1", "10.0.0.0/30", "[::1]", "db.internal", "", "a..b", "1.2.3.4/33", "::/120", "-x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, spec string) {
		got, err := Expand(context.Background(), resolver, []string{spec}, 64)
		if err != nil {
			if !errors.Is(err, ErrBadTarget) && !errors.Is(err, ErrTooManyHosts) && !errors.Is(err, ErrResolve) {
				t.Fatalf("unexpected error type: %v", err)
			}
			return
		}
		if len(got) == 0 || len(got) > 64 {
			t.Fatalf("got %d targets", len(got))
		}
		seen := map[netip.Addr]bool{}
		for _, tg := range got {
			if !tg.Addr.IsValid() || seen[tg.Addr] {
				t.Fatalf("invalid or duplicate target %v", tg)
			}
			seen[tg.Addr] = true
		}
	})
}
