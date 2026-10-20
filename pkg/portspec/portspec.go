// Package portspec parses and represents sets of TCP/UDP port numbers.
//
// A spec is a comma-separated list of single ports, inclusive ranges and named
// presets, for example:
//
//	80
//	22,80,443
//	8000-8100
//	common,9000-9010
//
// Parsed specs are normalized: ranges are sorted and overlapping or adjacent
// ranges are merged, so every port appears exactly once.
package portspec

import (
	"errors"
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
)

// MinPort and MaxPort bound valid port numbers. Port 0 is reserved and rejected.
const (
	MinPort = 1
	MaxPort = 65535
)

// Sentinel errors wrapped by *ParseError. Use errors.Is to test for them.
var (
	ErrEmpty          = errors.New("empty port spec")
	ErrInvalidPort    = errors.New("invalid port number")
	ErrOutOfRange     = errors.New("port out of range 1-65535")
	ErrReversedRange  = errors.New("range start is greater than end")
	ErrUnknownPreset  = errors.New("unknown preset")
	ErrMalformedRange = errors.New("malformed range")
)

// ParseError reports a problem with one element of a port spec.
type ParseError struct {
	Input string // the full spec that was parsed
	Token string // the offending element
	Err   error  // one of the sentinel errors above
}

func (e *ParseError) Error() string {
	if e.Token == "" {
		return fmt.Sprintf("portspec %q: %v", e.Input, e.Err)
	}
	return fmt.Sprintf("portspec %q: %q: %v", e.Input, e.Token, e.Err)
}

func (e *ParseError) Unwrap() error { return e.Err }

// Range is an inclusive range of ports. A single port has Start == End.
type Range struct {
	Start uint16
	End   uint16
}

// Len returns the number of ports in the range.
func (r Range) Len() int { return int(r.End) - int(r.Start) + 1 }

// Contains reports whether p lies within the range.
func (r Range) Contains(p uint16) bool { return p >= r.Start && p <= r.End }

// String returns "80" for a single port or "80-90" for a range.
func (r Range) String() string {
	if r.Start == r.End {
		return strconv.Itoa(int(r.Start))
	}
	return strconv.Itoa(int(r.Start)) + "-" + strconv.Itoa(int(r.End))
}

// Spec is a normalized, immutable set of ports. The zero value is empty.
type Spec struct {
	ranges []Range
}

// Parse parses a port spec. Whitespace around elements is ignored; empty
// elements (e.g. "80,,443") are rejected.
func Parse(s string) (Spec, error) {
	if strings.TrimSpace(s) == "" {
		return Spec{}, &ParseError{Input: s, Err: ErrEmpty}
	}
	var ranges []Range
	for tok := range strings.SplitSeq(s, ",") {
		tok = strings.TrimSpace(tok)
		r, err := parseToken(tok)
		if err != nil {
			return Spec{}, &ParseError{Input: s, Token: tok, Err: err}
		}
		ranges = append(ranges, r...)
	}
	return New(ranges...), nil
}

// MustParse is like Parse but panics on error. Intended for constants and tests.
func MustParse(s string) Spec {
	spec, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return spec
}

// New builds a normalized Spec from ranges. Ranges with Start > End or
// Start == 0 are invalid input; New swaps reversed bounds and clamps 0 to 1 so
// that the result is always well-formed.
func New(ranges ...Range) Spec {
	if len(ranges) == 0 {
		return Spec{}
	}
	rs := make([]Range, 0, len(ranges))
	for _, r := range ranges {
		if r.Start > r.End {
			r.Start, r.End = r.End, r.Start
		}
		if r.End == 0 {
			continue
		}
		r.Start = max(r.Start, MinPort)
		rs = append(rs, r)
	}
	slices.SortFunc(rs, func(a, b Range) int { return int(a.Start) - int(b.Start) })

	merged := rs[:0]
	for _, r := range rs {
		if n := len(merged); n > 0 && int(r.Start) <= int(merged[n-1].End)+1 {
			merged[n-1].End = max(merged[n-1].End, r.End)
			continue
		}
		merged = append(merged, r)
	}
	if len(merged) == 0 {
		return Spec{}
	}
	return Spec{ranges: merged}
}

// Single returns a Spec containing exactly one port.
func Single(p uint16) Spec { return New(Range{Start: p, End: p}) }

// ParsePort parses a single port number in the range 1-65535.
func ParsePort(s string) (uint16, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("%q: %w", s, ErrInvalidPort)
	}
	if n < MinPort || n > MaxPort {
		return 0, fmt.Errorf("%d: %w", n, ErrOutOfRange)
	}
	return uint16(n), nil
}

func parseToken(tok string) ([]Range, error) {
	if tok == "" {
		return nil, ErrEmpty
	}
	if p, ok := presets[strings.ToLower(tok)]; ok {
		return p, nil
	}
	if lo, hi, ok := strings.Cut(tok, "-"); ok {
		lo, hi = strings.TrimSpace(lo), strings.TrimSpace(hi)
		if lo == "" || hi == "" || strings.Contains(hi, "-") {
			return nil, ErrMalformedRange
		}
		start, err := parseNumber(lo)
		if err != nil {
			return nil, err
		}
		end, err := parseNumber(hi)
		if err != nil {
			return nil, err
		}
		if start > end {
			return nil, ErrReversedRange
		}
		return []Range{{Start: start, End: end}}, nil
	}
	if !isDigits(tok) {
		return nil, ErrUnknownPreset
	}
	p, err := parseNumber(tok)
	if err != nil {
		return nil, err
	}
	return []Range{{Start: p, End: p}}, nil
}

func parseNumber(s string) (uint16, error) {
	if !isDigits(s) {
		return 0, ErrInvalidPort
	}
	// Reject absurdly long inputs before conversion to avoid overflow surprises.
	if len(strings.TrimLeft(s, "0")) > 5 {
		return 0, ErrOutOfRange
	}
	n, _ := strconv.Atoi(s) // cannot fail: digits only, at most 5 significant
	if n < MinPort || n > MaxPort {
		return 0, ErrOutOfRange
	}
	return uint16(n), nil
}

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// Ranges returns a copy of the normalized ranges.
func (s Spec) Ranges() []Range { return slices.Clone(s.ranges) }

// IsEmpty reports whether the spec contains no ports.
func (s Spec) IsEmpty() bool { return len(s.ranges) == 0 }

// Len returns the total number of ports in the spec.
func (s Spec) Len() int {
	n := 0
	for _, r := range s.ranges {
		n += r.Len()
	}
	return n
}

// Contains reports whether p is in the spec.
func (s Spec) Contains(p uint16) bool {
	_, found := slices.BinarySearchFunc(s.ranges, p, func(r Range, p uint16) int {
		switch {
		case r.End < p:
			return -1
		case r.Start > p:
			return 1
		default:
			return 0
		}
	})
	return found
}

// At returns the i-th port (0-based, ascending). ok is false if i is out of bounds.
func (s Spec) At(i int) (port uint16, ok bool) {
	if i < 0 {
		return 0, false
	}
	for _, r := range s.ranges {
		if i < r.Len() {
			return uint16(int(r.Start) + i), true
		}
		i -= r.Len()
	}
	return 0, false
}

// All yields every port in ascending order.
func (s Spec) All() iter.Seq[uint16] {
	return func(yield func(uint16) bool) {
		for _, r := range s.ranges {
			for p := int(r.Start); p <= int(r.End); p++ {
				if !yield(uint16(p)) {
					return
				}
			}
		}
	}
}

// String returns the canonical form, e.g. "22,80-81,443". It round-trips through Parse.
func (s Spec) String() string {
	parts := make([]string, len(s.ranges))
	for i, r := range s.ranges {
		parts[i] = r.String()
	}
	return strings.Join(parts, ",")
}
