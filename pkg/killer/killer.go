// Package killer terminates processes safely.
//
// Kill first asks the process to exit (SIGTERM on Unix), waits for a grace
// period, and only then force-kills it (SIGKILL). On Windows there is no
// portable graceful signal, so both steps use TerminateProcess.
//
// Guards refuse to act on PID 0/1, the calling process and its parent; well
// known system processes are refused unless Options.AllowProtected is set.
package killer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Errors returned (wrapped) by Kill.
var (
	ErrProtected    = errors.New("refusing to kill protected process")
	ErrNotFound     = errors.New("process not found")
	ErrPermission   = errors.New("permission denied")
	ErrStillRunning = errors.New("process still running after kill")
)

// Outcome describes how a Kill call ended.
type Outcome int

// Possible outcomes.
const (
	// Terminated means the process exited after the graceful request.
	Terminated Outcome = iota
	// Killed means the process had to be force-killed.
	Killed
	// AlreadyExited means the process exited before it could be signalled.
	AlreadyExited
	// DryRun means no signal was sent because Options.DryRun was set.
	DryRun
)

func (o Outcome) String() string {
	switch o {
	case Terminated:
		return "terminated"
	case Killed:
		return "killed"
	case AlreadyExited:
		return "already-exited"
	case DryRun:
		return "dry-run"
	default:
		return "unknown(" + strconv.Itoa(int(o)) + ")"
	}
}

// Options control Kill.
type Options struct {
	// Grace is how long to wait after the graceful request before force
	// killing. Zero means DefaultGrace.
	Grace time.Duration
	// Force skips the graceful request and kills immediately.
	Force bool
	// AllowProtected permits killing well-known system processes. It never
	// permits PID 0/1, the caller itself or its parent.
	AllowProtected bool
	// DryRun runs all checks but sends no signal.
	DryRun bool
}

// Result reports what Kill did.
type Result struct {
	PID     int32
	Name    string
	Outcome Outcome
}

// Default timings.
const (
	DefaultGrace = 5 * time.Second
	killWait     = 3 * time.Second
	pollEvery    = 50 * time.Millisecond
)

// ops is the OS boundary, replaced by a fake in unit tests.
type ops interface {
	Name(ctx context.Context, pid int32) (string, error)
	Terminate(ctx context.Context, pid int32) error
	Kill(ctx context.Context, pid int32) error
	Alive(ctx context.Context, pid int32) bool
}

// Killer terminates processes. The zero value is not usable; use New.
type Killer struct {
	ops       ops
	self      int32
	parent    int32
	killWait  time.Duration
	pollEvery time.Duration
}

// New returns a Killer that acts on real processes.
func New() *Killer {
	return &Killer{
		ops:       systemOps{},
		self:      int32(os.Getpid()),
		parent:    int32(os.Getppid()),
		killWait:  killWait,
		pollEvery: pollEvery,
	}
}

// protectedNames are processes whose termination would likely crash or lock
// the system or the user session. Compared case-insensitively.
var protectedNames = []string{
	// macOS
	"launchd", "kernel_task", "windowserver", "loginwindow", "logd", "configd",
	// Linux
	"systemd", "init", "kthreadd", "dbus-daemon",
	// Windows
	"system", "idle", "registry", "smss.exe", "csrss.exe", "wininit.exe",
	"winlogon.exe", "services.exe", "lsass.exe", "svchost.exe", "dwm.exe",
}

// IsProtectedName reports whether name is a protected system process.
func IsProtectedName(name string) bool {
	return slices.Contains(protectedNames, strings.ToLower(strings.TrimSpace(name)))
}

// Check runs the safety guards for pid without signalling it.
func (k *Killer) Check(ctx context.Context, pid int32, allowProtected bool) (name string, err error) {
	switch {
	case pid <= 1:
		return "", fmt.Errorf("%w: pid %d is reserved", ErrProtected, pid)
	case pid == k.self:
		return "", fmt.Errorf("%w: pid %d is portcheck itself", ErrProtected, pid)
	case pid == k.parent:
		return "", fmt.Errorf("%w: pid %d is portcheck's parent (your shell?)", ErrProtected, pid)
	}
	name, err = k.ops.Name(ctx, pid)
	if err != nil {
		return "", fmt.Errorf("pid %d: %w", pid, err)
	}
	if !allowProtected && IsProtectedName(name) {
		return name, fmt.Errorf("%w: %q (pid %d) is a system process", ErrProtected, name, pid)
	}
	return name, nil
}

// Kill terminates pid according to opts.
func (k *Killer) Kill(ctx context.Context, pid int32, opts Options) (Result, error) {
	name, err := k.Check(ctx, pid, opts.AllowProtected)
	res := Result{PID: pid, Name: name}
	if err != nil {
		return res, err
	}
	if opts.DryRun {
		res.Outcome = DryRun
		return res, nil
	}

	if !opts.Force {
		grace := opts.Grace
		if grace <= 0 {
			grace = DefaultGrace
		}
		if err := k.ops.Terminate(ctx, pid); err != nil {
			if errors.Is(err, ErrNotFound) {
				res.Outcome = AlreadyExited
				return res, nil
			}
			return res, fmt.Errorf("terminate pid %d: %w", pid, err)
		}
		exited, err := k.waitExit(ctx, pid, grace)
		if err != nil {
			return res, err
		}
		if exited {
			res.Outcome = Terminated
			return res, nil
		}
	}

	if err := k.ops.Kill(ctx, pid); err != nil {
		if errors.Is(err, ErrNotFound) {
			// It exited between the grace check and the kill.
			res.Outcome = Terminated
			if opts.Force {
				res.Outcome = AlreadyExited
			}
			return res, nil
		}
		return res, fmt.Errorf("kill pid %d: %w", pid, err)
	}
	exited, err := k.waitExit(ctx, pid, k.killWait)
	if err != nil {
		return res, err
	}
	if !exited {
		return res, fmt.Errorf("pid %d: %w", pid, ErrStillRunning)
	}
	res.Outcome = Killed
	return res, nil
}

// waitExit polls until pid is gone, the timeout elapses (false, nil) or ctx is done.
func (k *Killer) waitExit(ctx context.Context, pid int32, timeout time.Duration) (bool, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(k.pollEvery)
	defer tick.Stop()
	for {
		if !k.ops.Alive(ctx, pid) {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, fmt.Errorf("waiting for pid %d: %w", pid, ctx.Err())
		case <-deadline.C:
			return false, nil
		case <-tick.C:
		}
	}
}
