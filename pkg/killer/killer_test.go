package killer

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// fakeOps simulates one process. ignoreTerm makes it survive SIGTERM;
// ignoreKill makes it survive SIGKILL too (e.g. stuck in uninterruptible I/O).
type fakeOps struct {
	mu                     sync.Mutex
	name                   string
	nameErr                error
	alive                  bool
	ignoreTerm, ignoreKill bool
	termErr, killErr       error
	calls                  []string
	// exitAfterAliveChecks makes the process exit on its own after N Alive calls.
	exitAfterAliveChecks int
}

func (f *fakeOps) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakeOps) Name(context.Context, int32) (string, error) {
	f.record("name")
	return f.name, f.nameErr
}

func (f *fakeOps) Terminate(context.Context, int32) error {
	f.record("term")
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.termErr != nil {
		return f.termErr
	}
	if !f.ignoreTerm {
		f.alive = false
	}
	return nil
}

func (f *fakeOps) Kill(context.Context, int32) error {
	f.record("kill")
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.killErr != nil {
		return f.killErr
	}
	if !f.ignoreKill {
		f.alive = false
	}
	return nil
}

func (f *fakeOps) Alive(context.Context, int32) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.exitAfterAliveChecks > 0 {
		f.exitAfterAliveChecks--
		if f.exitAfterAliveChecks == 0 {
			f.alive = false
		}
	}
	return f.alive
}

func (f *fakeOps) callString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, ",")
}

func testKiller(o *fakeOps) *Killer {
	return &Killer{ops: o, self: 1000, parent: 999, killWait: 30 * time.Millisecond, pollEvery: time.Millisecond}
}

const grace = 30 * time.Millisecond

func TestKillOutcomes(t *testing.T) {
	t.Parallel()
	notFound := errors.Join(ErrNotFound, errors.New("gone"))
	tests := []struct {
		name      string
		ops       *fakeOps
		opts      Options
		want      Outcome
		wantErr   error
		wantCalls string
	}{
		{name: "graceful", ops: &fakeOps{name: "node", alive: true}, want: Terminated, wantCalls: "name,term"},
		{name: "needs force", ops: &fakeOps{name: "node", alive: true, ignoreTerm: true}, want: Killed, wantCalls: "name,term,kill"},
		{name: "force skips term", ops: &fakeOps{name: "node", alive: true}, opts: Options{Force: true}, want: Killed, wantCalls: "name,kill"},
		{name: "dry run sends nothing", ops: &fakeOps{name: "node", alive: true}, opts: Options{DryRun: true}, want: DryRun, wantCalls: "name"},
		{name: "exits before term", ops: &fakeOps{name: "node", termErr: notFound}, want: AlreadyExited, wantCalls: "name,term"},
		{name: "exits between term and kill", ops: &fakeOps{name: "node", alive: true, ignoreTerm: true, killErr: notFound}, want: Terminated, wantCalls: "name,term,kill"},
		{name: "exits before forced kill", ops: &fakeOps{name: "node", killErr: notFound}, opts: Options{Force: true}, want: AlreadyExited, wantCalls: "name,kill"},
		{name: "exits on its own during grace", ops: &fakeOps{name: "node", alive: true, ignoreTerm: true, exitAfterAliveChecks: 3}, want: Terminated, wantCalls: "name,term"},
		{name: "unkillable", ops: &fakeOps{name: "node", alive: true, ignoreTerm: true, ignoreKill: true}, want: Terminated, wantErr: ErrStillRunning, wantCalls: "name,term,kill"},
		{name: "term permission", ops: &fakeOps{name: "node", alive: true, termErr: errors.Join(ErrPermission, fs.ErrPermission)}, wantErr: ErrPermission, wantCalls: "name,term"},
		{name: "kill permission", ops: &fakeOps{name: "node", alive: true, killErr: ErrPermission}, opts: Options{Force: true}, wantErr: ErrPermission, wantCalls: "name,kill"},
		{name: "name lookup fails", ops: &fakeOps{nameErr: notFound}, wantErr: ErrNotFound, wantCalls: "name"},
		{name: "protected name", ops: &fakeOps{name: "launchd", alive: true}, wantErr: ErrProtected, wantCalls: "name"},
		{name: "protected allowed", ops: &fakeOps{name: "svchost.exe", alive: true}, opts: Options{AllowProtected: true}, want: Terminated, wantCalls: "name,term"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.opts.Grace == 0 {
				tt.opts.Grace = grace
			}
			res, err := testKiller(tt.ops).Kill(t.Context(), 4242, tt.opts)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res.Outcome != tt.want {
					t.Errorf("outcome = %v, want %v", res.Outcome, tt.want)
				}
			}
			if res.PID != 4242 {
				t.Errorf("PID = %d", res.PID)
			}
			if got := tt.ops.callString(); got != tt.wantCalls {
				t.Errorf("calls = %q, want %q", got, tt.wantCalls)
			}
		})
	}
}

func TestHardGuards(t *testing.T) {
	t.Parallel()
	for _, pid := range []int32{-5, 0, 1, 1000, 999} {
		o := &fakeOps{name: "node", alive: true}
		_, err := testKiller(o).Kill(t.Context(), pid, Options{AllowProtected: true, Force: true})
		if !errors.Is(err, ErrProtected) {
			t.Errorf("pid %d: error = %v, want ErrProtected", pid, err)
		}
		if o.callString() != "" {
			t.Errorf("pid %d: ops called (%s) despite hard guard", pid, o.callString())
		}
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	name, err := testKiller(&fakeOps{name: "nginx"}).Check(t.Context(), 50, false)
	if err != nil || name != "nginx" {
		t.Errorf("Check = %q, %v", name, err)
	}
	name, err = testKiller(&fakeOps{name: "systemd"}).Check(t.Context(), 50, false)
	if !errors.Is(err, ErrProtected) || name != "systemd" {
		t.Errorf("Check protected = %q, %v", name, err)
	}
}

func TestDefaultGrace(t *testing.T) {
	t.Parallel()
	// With Grace unset the default (5s) applies; the process exits on its
	// own quickly, so the test stays fast while exercising the default path.
	o := &fakeOps{name: "x", alive: true, ignoreTerm: true, exitAfterAliveChecks: 2}
	res, err := testKiller(o).Kill(t.Context(), 4242, Options{})
	if err != nil || res.Outcome != Terminated {
		t.Errorf("Kill = %+v, %v", res, err)
	}
}

func TestContextCanceledWhileWaiting(t *testing.T) {
	t.Parallel()
	for _, opts := range []Options{{Grace: time.Hour}, {Force: true}} {
		ctx, cancel := context.WithCancel(t.Context())
		o := &fakeOps{name: "x", alive: true, ignoreTerm: true, ignoreKill: true}
		k := testKiller(o)
		k.killWait = time.Hour
		time.AfterFunc(20*time.Millisecond, cancel)
		_, err := k.Kill(ctx, 4242, opts)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("opts %+v: error = %v, want context.Canceled", opts, err)
		}
		cancel()
	}
}

func TestIsProtectedName(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{"launchd": true, "SystemD": true, "LSASS.EXE": true, "node": false, "": false, "launchd2": false} {
		if got := IsProtectedName(name); got != want {
			t.Errorf("IsProtectedName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestOutcomeString(t *testing.T) {
	t.Parallel()
	for o, want := range map[Outcome]string{Terminated: "terminated", Killed: "killed", AlreadyExited: "already-exited", DryRun: "dry-run", Outcome(42): "unknown(42)"} {
		if o.String() != want {
			t.Errorf("%d.String() = %q, want %q", int(o), o.String(), want)
		}
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	k := New()
	if k.self != int32(os.Getpid()) || k.parent != int32(os.Getppid()) {
		t.Errorf("self/parent = %d/%d", k.self, k.parent)
	}
	if k.killWait != killWait || k.pollEvery != pollEvery {
		t.Error("timings not defaulted")
	}
	if _, ok := k.ops.(systemOps); !ok {
		t.Errorf("ops = %T, want systemOps", k.ops)
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()
	if classify(nil) != nil {
		t.Error("classify(nil) != nil")
	}
	for _, err := range []error{process.ErrorProcessNotRunning, os.ErrProcessDone, syscall.ESRCH} {
		if got := classify(err); !errors.Is(got, ErrNotFound) || !errors.Is(got, err) {
			t.Errorf("classify(%v) = %v, want ErrNotFound wrapping original", err, got)
		}
	}
	if got := classify(fs.ErrPermission); !errors.Is(got, ErrPermission) {
		t.Errorf("classify(ErrPermission) = %v", got)
	}
	other := errors.New("other")
	if got := classify(other); got != other { //nolint:errorlint // identity is the point of this assertion
		t.Errorf("classify(other) = %v", got)
	}
}

func TestSystemOpsSelf(t *testing.T) {
	t.Parallel()
	ops := systemOps{}
	self := int32(os.Getpid())
	name, err := ops.Name(t.Context(), self)
	if err != nil || name == "" {
		t.Errorf("Name(self) = %q, %v", name, err)
	}
	if !ops.Alive(t.Context(), self) {
		t.Error("Alive(self) = false")
	}
}
