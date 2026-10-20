package killer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"syscall"

	"github.com/shirou/gopsutil/v4/process"
)

// systemOps implements ops with gopsutil.
type systemOps struct{}

func (systemOps) Name(ctx context.Context, pid int32) (string, error) {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err == nil {
		var name string
		if name, err = p.NameWithContext(ctx); err == nil {
			return name, nil
		}
	}
	return "", classify(err)
}

func (systemOps) Terminate(ctx context.Context, pid int32) error {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return classify(err)
	}
	return classify(p.TerminateWithContext(ctx))
}

func (systemOps) Kill(ctx context.Context, pid int32) error {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return classify(err)
	}
	return classify(p.KillWithContext(ctx))
}

// Alive reports whether pid exists and is not a zombie (an exited process
// whose parent has not reaped it yet).
func (systemOps) Alive(ctx context.Context, pid int32) bool {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return false
	}
	if st, err := p.StatusWithContext(ctx); err == nil && slices.Contains(st, process.Zombie) {
		return false
	}
	return true
}

// classify maps OS errors to this package's sentinels, keeping the original.
func classify(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, process.ErrorProcessNotRunning),
		errors.Is(err, os.ErrProcessDone),
		errors.Is(err, syscall.ESRCH):
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%w: %w", ErrPermission, err)
	default:
		return err
	}
}
