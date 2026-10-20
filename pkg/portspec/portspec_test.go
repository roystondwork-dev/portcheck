package portspec

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want []Range
	}{
		{"80", []Range{{80, 80}}},
		{" 80 ", []Range{{80, 80}}},
		{"1", []Range{{1, 1}}},
		{"65535", []Range{{65535, 65535}}},
		{"0080", []Range{{80, 80}}},
		{"443,80,22", []Range{{22, 22}, {80, 80}, {443, 443}}},
		{"8000-8100", []Range{{8000, 8100}}},
		{"8000 - 8100", []Range{{8000, 8100}}},
		{"5-5", []Range{{5, 5}}},
		{"80,81,82", []Range{{80, 82}}},                   // adjacent singles merge
		{"1-10,5-20", []Range{{1, 20}}},                   // overlap merges
		{"1-10,11-20", []Range{{1, 20}}},                  // adjacency merges
		{"1-10,12-20", []Range{{1, 10}, {12, 20}}},        // gap preserved
		{"100-200,150,50", []Range{{50, 50}, {100, 200}}}, // contained single absorbed
		{"well-known", []Range{{1, 1023}}},
		{"WELL-KNOWN", []Range{{1, 1023}}},
		{"registered", []Range{{1024, 49151}}},
		{"dynamic", []Range{{49152, 65535}}},
		{"all", []Range{{1, 65535}}},
		{"well-known,registered,dynamic", []Range{{1, 65535}}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.in, err)
			}
			if diff := cmp.Diff(tt.want, got.Ranges()); diff != "" {
				t.Errorf("Parse(%q) ranges (-want +got):\n%s", tt.in, diff)
			}
		})
	}
}

func TestParseCommonPreset(t *testing.T) {
	t.Parallel()
	s := MustParse("common")
	if s.Len() != len(commonPorts) {
		t.Errorf("common preset has %d ports, want %d (duplicates in list?)", s.Len(), len(commonPorts))
	}
	for _, p := range []uint16{22, 80, 443, 3000, 5432, 6379, 8080} {
		if !s.Contains(p) {
			t.Errorf("common preset missing port %d", p)
		}
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		token   string
		wantErr error
	}{
		{"", "", ErrEmpty},
		{"   ", "", ErrEmpty},
		{"80,,443", "", ErrEmpty},
		{"80,", "", ErrEmpty},
		{"0", "0", ErrOutOfRange},
		{"65536", "65536", ErrOutOfRange},
		{"99999999999999999999", "99999999999999999999", ErrOutOfRange},
		{"0-10", "0-10", ErrOutOfRange},
		{"10-70000", "10-70000", ErrOutOfRange},
		{"100-10", "100-10", ErrReversedRange},
		{"-10", "-10", ErrMalformedRange},
		{"10-", "10-", ErrMalformedRange},
		{"1-2-3", "1-2-3", ErrMalformedRange},
		{"a-10", "a-10", ErrInvalidPort},
		{"10-b", "10-b", ErrInvalidPort},
		{"http", "http", ErrUnknownPreset},
		{"+80", "+80", ErrUnknownPreset},
		{"8 0", "8 0", ErrUnknownPreset},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Parse(%q) error = %v, want %v", tt.in, err, tt.wantErr)
			}
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("Parse(%q) error type = %T, want *ParseError", tt.in, err)
			}
			if pe.Input != tt.in || pe.Token != tt.token {
				t.Errorf("ParseError{Input:%q Token:%q}, want {Input:%q Token:%q}", pe.Input, pe.Token, tt.in, tt.token)
			}
		})
	}
}

func TestParseErrorMessage(t *testing.T) {
	t.Parallel()
	_, err := Parse("80,abc")
	if got, want := err.Error(), `portspec "80,abc": "abc": unknown preset`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	_, err = Parse("")
	if got, want := err.Error(), `portspec "": empty port spec`; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestMustParse(t *testing.T) {
	t.Parallel()
	if got := MustParse("22,80").String(); got != "22,80" {
		t.Errorf("MustParse = %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("MustParse did not panic on invalid input")
		}
	}()
	MustParse("nope")
}

func TestNew(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []Range
		want []Range
	}{
		{"nil", nil, nil},
		{"reversed is swapped", []Range{{20, 10}}, []Range{{10, 20}}},
		{"zero start clamped", []Range{{0, 5}}, []Range{{1, 5}}},
		{"zero-only dropped", []Range{{0, 0}}, nil},
		{"zero-only dropped among others", []Range{{0, 0}, {7, 7}}, []Range{{7, 7}}},
		{"max boundary merge", []Range{{65535, 65535}, {65530, 65534}}, []Range{{65530, 65535}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if diff := cmp.Diff(tt.want, New(tt.in...).Ranges()); diff != "" {
				t.Errorf("New ranges (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewDoesNotAliasInput(t *testing.T) {
	t.Parallel()
	in := []Range{{30, 40}, {1, 2}}
	_ = New(in...)
	if diff := cmp.Diff([]Range{{30, 40}, {1, 2}}, in); diff != "" {
		t.Errorf("New mutated its input (-want +got):\n%s", diff)
	}
}

func TestSingle(t *testing.T) {
	t.Parallel()
	if diff := cmp.Diff([]Range{{443, 443}}, Single(443).Ranges()); diff != "" {
		t.Error(diff)
	}
}

func TestParsePort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    uint16
		wantErr error
	}{
		{"80", 80, nil},
		{" 8080 ", 8080, nil},
		{"65535", 65535, nil},
		{"0", 0, ErrOutOfRange},
		{"65536", 0, ErrOutOfRange},
		{"-1", 0, ErrOutOfRange},
		{"http", 0, ErrInvalidPort},
		{"", 0, ErrInvalidPort},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := ParsePort(tt.in)
			if !errors.Is(err, tt.wantErr) || got != tt.want {
				t.Errorf("ParsePort(%q) = %d, %v; want %d, %v", tt.in, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestRange(t *testing.T) {
	t.Parallel()
	r := Range{Start: 10, End: 12}
	if r.Len() != 3 {
		t.Errorf("Len = %d, want 3", r.Len())
	}
	if !r.Contains(10) || !r.Contains(12) || r.Contains(9) || r.Contains(13) {
		t.Error("Contains boundaries wrong")
	}
	if r.String() != "10-12" {
		t.Errorf("String = %q", r.String())
	}
	if (Range{Start: 7, End: 7}).String() != "7" {
		t.Error("single-port String wrong")
	}
	if (Range{Start: 1, End: 65535}).Len() != 65535 {
		t.Error("full range Len wrong")
	}
}

func TestSpecQueries(t *testing.T) {
	t.Parallel()
	s := MustParse("22,80-82,443")

	if s.IsEmpty() {
		t.Error("IsEmpty = true")
	}
	if !(Spec{}).IsEmpty() {
		t.Error("zero Spec should be empty")
	}
	if s.Len() != 5 {
		t.Errorf("Len = %d, want 5", s.Len())
	}
	if (Spec{}).Len() != 0 {
		t.Error("zero Spec Len != 0")
	}
	for p, want := range map[uint16]bool{1: false, 22: true, 23: false, 79: false, 80: true, 81: true, 82: true, 83: false, 443: true, 444: false, 65535: false} {
		if got := s.Contains(p); got != want {
			t.Errorf("Contains(%d) = %v, want %v", p, got, want)
		}
	}
	if (Spec{}).Contains(80) {
		t.Error("zero Spec Contains(80)")
	}
	if got := s.String(); got != "22,80-82,443" {
		t.Errorf("String = %q", got)
	}
	if got := (Spec{}).String(); got != "" {
		t.Errorf("zero Spec String = %q", got)
	}
}

func TestRangesReturnsCopy(t *testing.T) {
	t.Parallel()
	s := MustParse("80")
	rs := s.Ranges()
	rs[0].Start = 1
	if s.Ranges()[0].Start != 80 {
		t.Error("Ranges() exposed internal slice")
	}
}

func TestAt(t *testing.T) {
	t.Parallel()
	s := MustParse("10-12,20")
	for i, want := range []uint16{10, 11, 12, 20} {
		if got, ok := s.At(i); !ok || got != want {
			t.Errorf("At(%d) = %d, %v; want %d, true", i, got, ok, want)
		}
	}
	for _, i := range []int{-1, 4, 100} {
		if got, ok := s.At(i); ok || got != 0 {
			t.Errorf("At(%d) = %d, %v; want 0, false", i, got, ok)
		}
	}
	if _, ok := (Spec{}).At(0); ok {
		t.Error("zero Spec At(0) ok")
	}
}

func TestAll(t *testing.T) {
	t.Parallel()
	got := slices.Collect(MustParse("3,1-2,10").All())
	if diff := cmp.Diff([]uint16{1, 2, 3, 10}, got); diff != "" {
		t.Errorf("All (-want +got):\n%s", diff)
	}

	// Early termination must stop iteration.
	var seen []uint16
	for p := range MustParse("1-100").All() {
		seen = append(seen, p)
		if p == 3 {
			break
		}
	}
	if diff := cmp.Diff([]uint16{1, 2, 3}, seen); diff != "" {
		t.Errorf("All with break (-want +got):\n%s", diff)
	}

	// The full range must not overflow uint16 iteration.
	n := 0
	for range MustParse("all").All() {
		n++
	}
	if n != 65535 {
		t.Errorf("all preset yielded %d ports, want 65535", n)
	}
}

func TestPresets(t *testing.T) {
	t.Parallel()
	want := []string{"all", "common", "dynamic", "registered", "well-known"}
	if diff := cmp.Diff(want, Presets()); diff != "" {
		t.Errorf("Presets (-want +got):\n%s", diff)
	}
	for _, name := range Presets() {
		if _, err := Parse(name); err != nil {
			t.Errorf("preset %q does not parse: %v", name, err)
		}
		if _, err := Parse(strings.ToUpper(name)); err != nil {
			t.Errorf("preset %q is not case-insensitive: %v", name, err)
		}
	}
}

// FuzzParse checks that Parse never panics, and that every successful parse is
// normalized and round-trips through String.
func FuzzParse(f *testing.F) {
	for _, seed := range []string{"80", "22,80,443", "8000-8100", "common", "1-10,5-20", "0", "65536", "a-b", ",,", "1-2-3", " 7 , 9-9 "} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		s, err := Parse(in)
		if err != nil {
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("error is %T, want *ParseError", err)
			}
			return
		}
		rs := s.Ranges()
		if len(rs) == 0 {
			t.Fatal("successful parse produced an empty spec")
		}
		for i, r := range rs {
			if r.Start < MinPort || r.Start > r.End {
				t.Fatalf("invalid range %+v", r)
			}
			if i > 0 && int(r.Start) <= int(rs[i-1].End)+1 {
				t.Fatalf("ranges not normalized: %+v", rs)
			}
		}
		again, err := Parse(s.String())
		if err != nil {
			t.Fatalf("canonical form %q does not re-parse: %v", s.String(), err)
		}
		if diff := cmp.Diff(rs, again.Ranges()); diff != "" {
			t.Fatalf("round trip changed ranges (-first +second):\n%s", diff)
		}
	})
}
