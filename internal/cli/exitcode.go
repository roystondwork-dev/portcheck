package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
)

// Exit codes. They are part of portcheck's public interface (docs/exit-codes.md).
const (
	// ExitOK means success: e.g. all checked ports are free.
	ExitOK = 0
	// ExitNegative means the command worked but the answer is "no": a port is
	// in use, no free port was found, nothing was killed.
	ExitNegative = 1
	// ExitUsage means invalid arguments or flags.
	ExitUsage = 2
	// ExitRuntime means an unexpected error.
	ExitRuntime = 3
	// ExitPermission means the operation needs more privileges.
	ExitPermission = 4
	// ExitInterrupted means the user pressed Ctrl-C (128 + SIGINT).
	ExitInterrupted = 130
)

// ExitError carries an explicit exit code. Silent errors are not printed
// because the command output already explains the result.
type ExitError struct {
	Code   int
	Err    error
	Silent bool
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit status %d", e.Code)
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

func usageError(format string, args ...any) error {
	return &ExitError{Code: ExitUsage, Err: fmt.Errorf(format, args...)}
}

// negative returns a silent ExitNegative error.
func negative() error { return &ExitError{Code: ExitNegative, Silent: true} }

// usageArgs marks cobra positional-argument errors as usage errors.
func usageArgs(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := v(cmd, args); err != nil {
			return &ExitError{Code: ExitUsage, Err: err}
		}
		return nil
	}
}

func isPermission(err error) bool {
	return errors.Is(err, inspect.ErrAccessDenied) || errors.Is(err, killer.ErrPermission) || errors.Is(err, fs.ErrPermission)
}

func exitCode(err error) int {
	var ee *ExitError
	switch {
	case err == nil:
		return ExitOK
	case errors.As(err, &ee):
		return ee.Code
	case errors.Is(err, context.Canceled):
		return ExitInterrupted
	case isPermission(err):
		return ExitPermission
	default:
		return ExitRuntime
	}
}

func isSilent(err error) bool {
	var ee *ExitError
	return (errors.As(err, &ee) && ee.Silent) || errors.Is(err, context.Canceled)
}
