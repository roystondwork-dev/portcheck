package inspect

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"strings"
	"syscall"
	"time"

	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// processHandle is the subset of *process.Process used by systemProvider.
type processHandle interface {
	NameWithContext(ctx context.Context) (string, error)
	UsernameWithContext(ctx context.Context) (string, error)
	CmdlineWithContext(ctx context.Context) (string, error)
	ExeWithContext(ctx context.Context) (string, error)
	CwdWithContext(ctx context.Context) (string, error)
	CreateTimeWithContext(ctx context.Context) (int64, error)
	PpidWithContext(ctx context.Context) (int32, error)
}

// systemProvider implements Provider using gopsutil, and on macOS a native
// libproc socket listing. See ADR-0001 and ADR-0003.
type systemProvider struct {
	// native, if set, lists sockets without gopsutil (macOS: libproc instead
	// of running lsof). On error, connections is used instead.
	native      func(ctx context.Context) ([]RawSocket, error)
	connections func(ctx context.Context, kind string) ([]gnet.ConnectionStat, error)
	newProcess  func(ctx context.Context, pid int32) (processHandle, error)
	// project names the project of a working directory; nil leaves it empty.
	project func(cwd string) string
}

// NewSystemProvider returns a Provider backed by the operating system.
//
// On macOS it reads sockets directly through libproc (the API lsof uses),
// falling back to lsof if that fails; without root it only sees sockets of the
// current user. On Linux and Windows, sockets of all users are listed but
// process details may require elevated privileges.
func NewSystemProvider() Provider {
	return &systemProvider{
		native:      nativeSockets,
		connections: gnet.ConnectionsWithContext,
		newProcess: func(ctx context.Context, pid int32) (processHandle, error) {
			return process.NewProcessWithContext(ctx, pid)
		},
		project: ProjectName,
	}
}

func (p *systemProvider) Sockets(ctx context.Context) ([]RawSocket, error) {
	if p.native != nil {
		if socks, err := p.native(ctx); err == nil {
			return socks, nil
		}
		// Fall through: the generic implementation still works.
	}
	stats, err := p.connections(ctx, "inet")
	if err != nil {
		return nil, fmt.Errorf("query connections: %w", classify(err))
	}
	out := make([]RawSocket, 0, len(stats))
	for _, c := range stats {
		if s, ok := fromConnectionStat(c); ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func (p *systemProvider) Process(ctx context.Context, pid int32) (*ProcessInfo, error) {
	h, err := p.newProcess(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("pid %d: %w", pid, classify(err))
	}
	name, err := h.NameWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("pid %d name: %w", pid, classify(err))
	}
	info := &ProcessInfo{PID: pid, Name: name}
	// The remaining fields are best effort: they commonly fail for processes
	// of other users without elevated privileges.
	if v, err := h.UsernameWithContext(ctx); err == nil {
		info.User = v
	}
	if v, err := h.CmdlineWithContext(ctx); err == nil {
		info.Cmdline = v
	}
	if v, err := h.ExeWithContext(ctx); err == nil {
		info.Exe = v
	}
	if ms, err := h.CreateTimeWithContext(ctx); err == nil && ms > 0 {
		info.CreateTime = time.UnixMilli(ms)
	}
	if v, err := h.CwdWithContext(ctx); err == nil {
		info.Cwd = v
		if p.project != nil {
			info.Project = p.project(v)
		}
	}
	info.Ancestors = p.ancestors(ctx, pid, h)
	return info, nil
}

// ancestors walks up the process tree from h, best effort.
func (p *systemProvider) ancestors(ctx context.Context, pid int32, h processHandle) []ProcessRef {
	var out []ProcessRef
	for range MaxAncestors {
		ppid, err := h.PpidWithContext(ctx)
		if err != nil || ppid <= 1 || ppid == pid {
			break // unknown, reached init/launchd, or a self-reference
		}
		parent, err := p.newProcess(ctx, ppid)
		if err != nil {
			break
		}
		name, err := parent.NameWithContext(ctx)
		if err != nil {
			break
		}
		out = append(out, ProcessRef{PID: ppid, Name: name})
		pid, h = ppid, parent
	}
	return out
}

// classify maps OS/gopsutil errors to this package's sentinel errors while
// keeping the original error in the chain.
func classify(err error) error {
	switch {
	case errors.Is(err, process.ErrorProcessNotRunning):
		return fmt.Errorf("%w: %w", ErrProcessNotFound, err)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%w: %w", ErrAccessDenied, err)
	default:
		return err
	}
}

func fromConnectionStat(c gnet.ConnectionStat) (RawSocket, bool) {
	var proto Proto
	switch c.Type {
	case syscall.SOCK_STREAM:
		proto = TCP
	case syscall.SOCK_DGRAM:
		proto = UDP
	default:
		return RawSocket{}, false
	}
	local, ok := toAddrPort(c.Laddr, c.Family)
	if !ok {
		return RawSocket{}, false
	}
	remote, _ := toAddrPort(c.Raddr, c.Family)
	if remote.Port() == 0 {
		// Linux and Windows report unconnected sockets as remote 0.0.0.0:0.
		remote = netip.AddrPort{}
	}
	return RawSocket{
		Proto:  proto,
		Local:  local,
		Remote: remote,
		State:  normalizeState(c.Status),
		PID:    max(c.Pid, 0),
	}, true
}

// toAddrPort converts a gopsutil address. lsof (macOS) reports wildcard
// addresses as "*", so the family decides between 0.0.0.0 and ::.
func toAddrPort(a gnet.Addr, family uint32) (netip.AddrPort, bool) {
	if (a.IP == "" && a.Port == 0) || a.Port > 65535 {
		return netip.AddrPort{}, false
	}
	var addr netip.Addr
	switch ip := strings.Trim(a.IP, "[]"); ip {
	case "", "*":
		addr = netip.IPv4Unspecified()
		if family == syscall.AF_INET6 {
			addr = netip.IPv6Unspecified()
		}
	default:
		parsed, err := netip.ParseAddr(ip)
		if err != nil {
			return netip.AddrPort{}, false
		}
		addr = parsed.Unmap()
	}
	return netip.AddrPortFrom(addr, uint16(a.Port)), true
}

func normalizeState(s string) State {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return StateNone
	}
	return State(s)
}
