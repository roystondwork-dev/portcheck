// Package inspect lists local network sockets together with the processes
// that own them.
//
// The OS-specific work is done by a Provider. NewSystem uses the default
// provider backed by gopsutil; tests and library users can supply their own
// (see FakeProvider). All filtering, de-duplication, process enrichment and
// ordering is done by Inspector and is identical on every platform.
package inspect

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// Proto is a transport protocol.
type Proto string

// Supported protocols.
const (
	TCP Proto = "tcp"
	UDP Proto = "udp"
)

// State is a socket state as reported by the OS, normalized to upper case
// (for example "LISTEN" or "ESTABLISHED"). Connectionless sockets report StateNone.
type State string

// Common socket states. Providers may report other values; they are passed through.
const (
	StateListen      State = "LISTEN"
	StateEstablished State = "ESTABLISHED"
	StateTimeWait    State = "TIME_WAIT"
	StateCloseWait   State = "CLOSE_WAIT"
	StateNone        State = "NONE"
)

// RawSocket is a socket as reported by a Provider, before process enrichment.
type RawSocket struct {
	Proto  Proto
	Local  netip.AddrPort
	Remote netip.AddrPort // zero value when the socket is not connected
	State  State
	PID    int32 // 0 when the owner is unknown (usually a permission limit)
}

// IsListening reports whether the socket accepts traffic: a TCP socket in
// LISTEN state, or an unconnected UDP socket.
func (s RawSocket) IsListening() bool {
	switch s.Proto {
	case TCP:
		return s.State == StateListen
	case UDP:
		return !s.Remote.IsValid() || s.Remote.Port() == 0
	default:
		return false
	}
}

// ProcessInfo describes the process that owns a socket. Fields other than PID
// and Name are best effort and may be empty without elevated privileges.
type ProcessInfo struct {
	PID        int32
	Name       string
	User       string
	Cmdline    string
	Exe        string
	CreateTime time.Time
}

// Container identifies a container that publishes a socket's port (for
// example through docker-proxy or Docker Desktop's backend).
type Container struct {
	ID      string
	Name    string
	Image   string
	Runtime string // e.g. "docker"
}

// Socket is a RawSocket enriched with information about its owning process
// and, when an Enricher is configured, the container publishing it.
type Socket struct {
	RawSocket
	Process   *ProcessInfo // nil when unknown
	Container *Container   // nil when the port is not published by a known container
}

// Enricher adds information to sockets after they are listed, e.g. which
// container publishes a port. Errors are ignored by Inspector: enrichment is
// best effort and must never make listing fail.
type Enricher interface {
	Enrich(ctx context.Context, sockets []Socket) error
}

// Port returns the local port.
func (s Socket) Port() uint16 { return s.Local.Port() }

// Filter selects sockets. Zero-valued fields match everything.
type Filter struct {
	Ports         portspec.Spec // local ports
	Protos        []Proto
	States        []State
	PIDs          []int32
	Name          string // case-insensitive glob on the process or container name, e.g. "node*"
	ListeningOnly bool
}

// ErrBadFilter is returned by List when the filter is invalid.
var ErrBadFilter = errors.New("invalid filter")

// Validate reports whether the filter is well-formed.
func (f Filter) Validate() error {
	if f.Name != "" {
		if _, err := path.Match(f.Name, ""); err != nil {
			return fmt.Errorf("%w: name pattern %q: %w", ErrBadFilter, f.Name, err)
		}
	}
	return nil
}

func (f Filter) matchRaw(s RawSocket) bool {
	if !f.Ports.IsEmpty() && !f.Ports.Contains(s.Local.Port()) {
		return false
	}
	if len(f.Protos) > 0 && !slices.Contains(f.Protos, s.Proto) {
		return false
	}
	if len(f.States) > 0 && !slices.Contains(f.States, s.State) {
		return false
	}
	if len(f.PIDs) > 0 && !slices.Contains(f.PIDs, s.PID) {
		return false
	}
	if f.ListeningOnly && !s.IsListening() {
		return false
	}
	return true
}

func (f Filter) matchName(s Socket) bool {
	if f.Name == "" {
		return true
	}
	pattern := strings.ToLower(f.Name)
	for _, name := range []string{processName(s.Process), containerName(s.Container)} {
		if name == "" {
			continue
		}
		if ok, _ := path.Match(pattern, strings.ToLower(name)); ok { // pattern validated in List
			return true
		}
	}
	return false
}

func processName(p *ProcessInfo) string {
	if p == nil {
		return ""
	}
	return p.Name
}

func containerName(c *Container) string {
	if c == nil {
		return ""
	}
	return c.Name
}

// Result is the outcome of Inspector.List.
type Result struct {
	Sockets []Socket
	// Partial is true when some owners could not be determined, typically
	// because portcheck lacks privileges. See PrivilegeHint.
	Partial bool
}

// Inspector lists sockets using a Provider.
type Inspector struct {
	provider      Provider
	concurrency   int
	enrichers     []Enricher
	enrichTimeout time.Duration
}

// Option configures an Inspector.
type Option func(*Inspector)

// WithConcurrency sets how many process lookups run in parallel (default 16).
// Values below 1 are ignored.
func WithConcurrency(n int) Option {
	return func(in *Inspector) {
		if n >= 1 {
			in.concurrency = n
		}
	}
}

// WithEnricher adds an Enricher that runs after sockets are listed. Each
// enricher gets at most 500ms; nil is ignored.
func WithEnricher(e Enricher) Option {
	return func(in *Inspector) {
		if e != nil {
			in.enrichers = append(in.enrichers, e)
		}
	}
}

// New returns an Inspector that uses p.
func New(p Provider, opts ...Option) *Inspector {
	in := &Inspector{provider: p, concurrency: 16, enrichTimeout: 500 * time.Millisecond}
	for _, o := range opts {
		o(in)
	}
	return in
}

// NewSystem returns an Inspector backed by the operating system.
func NewSystem(opts ...Option) *Inspector {
	return New(NewSystemProvider(), opts...)
}

// List returns the sockets matching f, sorted by local port, protocol, local
// address, remote address and PID.
func (in *Inspector) List(ctx context.Context, f Filter) (*Result, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	raw, err := in.provider.Sockets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list sockets: %w", err)
	}

	seen := make(map[RawSocket]struct{}, len(raw))
	candidates := make([]RawSocket, 0, len(raw))
	pidSet := make(map[int32]struct{})
	res := &Result{}
	for _, s := range raw {
		if !f.matchRaw(s) {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		candidates = append(candidates, s)
		if s.PID > 0 {
			pidSet[s.PID] = struct{}{}
		} else {
			res.Partial = true
		}
	}

	procs, denied, err := in.lookupProcesses(ctx, pidSet)
	if err != nil {
		return nil, err
	}
	res.Partial = res.Partial || denied

	all := make([]Socket, len(candidates))
	for i, s := range candidates {
		all[i] = Socket{RawSocket: s, Process: procs[s.PID]}
	}
	in.enrich(ctx, all)

	res.Sockets = make([]Socket, 0, len(all))
	for _, s := range all {
		if f.matchName(s) {
			res.Sockets = append(res.Sockets, s)
		}
	}
	slices.SortFunc(res.Sockets, compareSockets)
	return res, nil
}

// enrich runs every enricher with a bounded time budget, ignoring failures.
func (in *Inspector) enrich(ctx context.Context, sockets []Socket) {
	if len(sockets) == 0 {
		return
	}
	for _, e := range in.enrichers {
		ectx, cancel := context.WithTimeout(ctx, in.enrichTimeout)
		_ = e.Enrich(ectx, sockets) // best effort by design
		cancel()
	}
}

// lookupProcesses resolves each PID concurrently. Processes that exited in
// the meantime are silently skipped; access-denied errors set denied.
func (in *Inspector) lookupProcesses(ctx context.Context, pids map[int32]struct{}) (map[int32]*ProcessInfo, bool, error) {
	var (
		mu     sync.Mutex
		procs  = make(map[int32]*ProcessInfo, len(pids))
		denied bool
		g      errgroup.Group
	)
	g.SetLimit(in.concurrency)
	for pid := range pids {
		g.Go(func() error {
			if ctx.Err() != nil {
				return nil
			}
			p, err := in.provider.Process(ctx, pid)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				procs[pid] = p
			case errors.Is(err, ErrAccessDenied):
				denied = true
			}
			return nil
		})
	}
	_ = g.Wait() // goroutines never return errors
	if err := ctx.Err(); err != nil {
		return nil, false, fmt.Errorf("lookup processes: %w", err)
	}
	return procs, denied, nil
}

func compareSockets(a, b Socket) int {
	return cmp.Or(
		cmp.Compare(a.Local.Port(), b.Local.Port()),
		cmp.Compare(a.Proto, b.Proto),
		a.Local.Addr().Compare(b.Local.Addr()),
		a.Remote.Compare(b.Remote),
		cmp.Compare(a.PID, b.PID),
		cmp.Compare(a.State, b.State),
	)
}
