// Package freeport determines whether local ports are free and finds free ones.
//
// A port is reported free only if (1) no listening socket on it is visible to
// the inspector and (2) it can actually be bound on every checked address
// (by default 127.0.0.1, 0.0.0.0, ::1 and ::). Combining both checks avoids
// false positives on platforms where SO_REUSEADDR lets a wildcard bind succeed
// next to an existing specific-address listener.
//
// Results are inherently racy: another process may take a port right after it
// was reported free. Bind the port yourself as soon as possible.
package freeport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"strconv"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// Availability is the verdict for a single port.
type Availability int

// Possible availabilities.
const (
	// Free means the port can be bound on all checked addresses.
	Free Availability = iota
	// InUse means a socket already listens on the port.
	InUse
	// Denied means binding failed for lack of privileges (e.g. ports < 1024
	// on Linux, or Windows excluded port ranges).
	Denied
)

func (a Availability) String() string {
	switch a {
	case Free:
		return "free"
	case InUse:
		return "in-use"
	case Denied:
		return "denied"
	default:
		return "unknown(" + strconv.Itoa(int(a)) + ")"
	}
}

// Errors returned by Finder.
var (
	ErrNoFreePort       = errors.New("no free port found")
	ErrUnsupportedProto = errors.New("unsupported protocol")
)

// Status describes one checked port.
type Status struct {
	Port         uint16
	Proto        inspect.Proto
	Availability Availability
	// Holders lists listening sockets on the port. It can be empty even when
	// the port is in use, if the owner is not visible without privileges.
	Holders []inspect.Socket
}

// Free reports whether the port is free.
func (s Status) Free() bool { return s.Availability == Free }

// Lister lists sockets; *inspect.Inspector implements it.
type Lister interface {
	List(ctx context.Context, f inspect.Filter) (*inspect.Result, error)
}

// DefaultHosts are the addresses a port must be bindable on to count as free.
var DefaultHosts = []string{"127.0.0.1", "0.0.0.0", "::1", "::"}

// bindFunc tries to bind proto/addr and releases it immediately.
type bindFunc func(ctx context.Context, proto inspect.Proto, addr string) error

// Finder checks and finds free ports.
type Finder struct {
	lister Lister
	hosts  []string
	bind   bindFunc
	randN  func(n int) int
}

// Option configures a Finder.
type Option func(*Finder)

// WithLister sets the socket lister used to find holders. Passing nil disables
// the listener check and relies on binding alone.
func WithLister(l Lister) Option { return func(f *Finder) { f.lister = l } }

// WithHosts overrides the addresses checked by binding.
func WithHosts(hosts ...string) Option { return func(f *Finder) { f.hosts = hosts } }

// New returns a Finder. By default it uses inspect.NewSystem() and DefaultHosts.
func New(opts ...Option) *Finder {
	f := &Finder{
		lister: inspect.NewSystem(),
		hosts:  DefaultHosts,
		bind:   bindOS,
		randN:  rand.IntN,
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

// Check reports the availability of a single port.
func (f *Finder) Check(ctx context.Context, port uint16, proto inspect.Proto) (Status, error) {
	sts, err := f.CheckAll(ctx, portspec.Single(port), proto)
	if err != nil {
		return Status{}, err
	}
	return sts[0], nil
}

// CheckAll reports the availability of every port in spec, in ascending
// order. Sockets are listed only once, so this is much cheaper than calling
// Check per port.
func (f *Finder) CheckAll(ctx context.Context, spec portspec.Spec, proto inspect.Proto) ([]Status, error) {
	if err := validProto(proto); err != nil {
		return nil, err
	}
	holders, err := f.holders(ctx, spec, proto)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, spec.Len())
	for p := range spec.All() {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("check ports: %w", err)
		}
		st := Status{Port: p, Proto: proto, Holders: holders[p]}
		if len(st.Holders) > 0 {
			st.Availability = InUse
		} else {
			st.Availability = f.bindable(ctx, proto, p)
		}
		out = append(out, st)
	}
	return out, nil
}

// Find returns up to n free ports from spec in ascending order. If fewer than
// n are found, it returns those it found together with an error wrapping
// ErrNoFreePort. n < 1 is treated as 1.
func (f *Finder) Find(ctx context.Context, spec portspec.Spec, proto inspect.Proto, n int) ([]uint16, error) {
	if err := validProto(proto); err != nil {
		return nil, err
	}
	n = max(n, 1)
	holders, err := f.holders(ctx, spec, proto)
	if err != nil {
		return nil, err
	}
	var out []uint16
	for p := range spec.All() {
		if err := ctx.Err(); err != nil {
			return out, fmt.Errorf("find free port: %w", err)
		}
		if len(holders[p]) == 0 && f.bindable(ctx, proto, p) == Free {
			out = append(out, p)
			if len(out) == n {
				return out, nil
			}
		}
	}
	return out, fmt.Errorf("found %d of %d in %s: %w", len(out), n, spec, ErrNoFreePort)
}

// maxRandomAttempts bounds how many random ports Random tries.
const maxRandomAttempts = 128

// Random returns a random free port from spec. It tries up to 128 random
// candidates (with possible repeats) before giving up.
func (f *Finder) Random(ctx context.Context, spec portspec.Spec, proto inspect.Proto) (uint16, error) {
	if err := validProto(proto); err != nil {
		return 0, err
	}
	total := spec.Len()
	if total == 0 {
		return 0, fmt.Errorf("empty spec: %w", ErrNoFreePort)
	}
	holders, err := f.holders(ctx, spec, proto)
	if err != nil {
		return 0, err
	}
	for range min(total, maxRandomAttempts) {
		if err := ctx.Err(); err != nil {
			return 0, fmt.Errorf("random free port: %w", err)
		}
		p, _ := spec.At(f.randN(total)) // index always in range
		if len(holders[p]) == 0 && f.bindable(ctx, proto, p) == Free {
			return p, nil
		}
	}
	return 0, fmt.Errorf("in %s: %w", spec, ErrNoFreePort)
}

func validProto(p inspect.Proto) error {
	if p != inspect.TCP && p != inspect.UDP {
		return fmt.Errorf("%w: %q", ErrUnsupportedProto, p)
	}
	return nil
}

func (f *Finder) holders(ctx context.Context, spec portspec.Spec, proto inspect.Proto) (map[uint16][]inspect.Socket, error) {
	out := make(map[uint16][]inspect.Socket)
	if f.lister == nil {
		return out, nil
	}
	res, err := f.lister.List(ctx, inspect.Filter{Ports: spec, Protos: []inspect.Proto{proto}, ListeningOnly: true})
	if err != nil {
		return nil, fmt.Errorf("find port holders: %w", err)
	}
	for _, s := range res.Sockets {
		out[s.Port()] = append(out[s.Port()], s)
	}
	return out, nil
}

// bindable tries every host; the first non-free verdict wins. Addresses the
// system cannot use (e.g. ::1 without IPv6) are skipped.
func (f *Finder) bindable(ctx context.Context, proto inspect.Proto, port uint16) Availability {
	for _, h := range f.hosts {
		err := f.bind(ctx, proto, net.JoinHostPort(h, strconv.Itoa(int(port))))
		switch classifyBindErr(err) {
		case bindOK, bindSkip:
			continue
		case bindInUse:
			return InUse
		case bindDenied:
			return Denied
		}
	}
	return Free
}

func bindOS(ctx context.Context, proto inspect.Proto, addr string) error {
	var (
		lc  net.ListenConfig
		c   io.Closer
		err error
	)
	if proto == inspect.UDP {
		c, err = lc.ListenPacket(ctx, "udp", addr)
	} else {
		c, err = lc.Listen(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("bind %s %s: %w", proto, addr, err)
	}
	_ = c.Close() // the bind succeeded; a close error does not change the verdict
	return nil
}
