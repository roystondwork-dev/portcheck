// Package cli implements the portcheck command line interface.
//
// Commands only parse arguments, call into pkg/*, and render results through
// internal/output. All OS dependencies are reached through App so that every
// command can be tested in-process with fakes.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/roystondwork-dev/portcheck/internal/buildinfo"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// SocketLister lists sockets; implemented by *inspect.Inspector.
type SocketLister interface {
	List(ctx context.Context, f inspect.Filter) (*inspect.Result, error)
}

// PortFinder checks and finds free ports; implemented by *freeport.Finder.
type PortFinder interface {
	CheckAll(ctx context.Context, spec portspec.Spec, proto inspect.Proto) ([]freeport.Status, error)
	Find(ctx context.Context, spec portspec.Spec, proto inspect.Proto, n int) ([]uint16, error)
	Random(ctx context.Context, spec portspec.Spec, proto inspect.Proto) (uint16, error)
}

// ProcessKiller terminates processes; implemented by *killer.Killer.
type ProcessKiller interface {
	Check(ctx context.Context, pid int32, allowProtected bool) (string, error)
	Kill(ctx context.Context, pid int32, opts killer.Options) (killer.Result, error)
}

// App bundles everything a command needs from the outside world.
type App struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Environ        []string
	// StdinTTY and StdoutTTY report whether the streams are terminals.
	StdinTTY, StdoutTTY bool

	Lister        SocketLister
	Ports         PortFinder
	Killer        ProcessKiller
	PrivilegeHint func() string
	Build         buildinfo.Info
}

// NewApp returns an App wired to the real operating system.
func NewApp() *App {
	lister := inspect.NewSystem()
	return &App{
		Stdin:         os.Stdin,
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
		Environ:       os.Environ(),
		StdinTTY:      term.IsTerminal(int(os.Stdin.Fd())),
		StdoutTTY:     term.IsTerminal(int(os.Stdout.Fd())),
		Lister:        lister,
		Ports:         freeport.New(freeport.WithLister(lister)),
		Killer:        killer.New(),
		PrivilegeHint: inspect.PrivilegeHint,
		Build:         buildinfo.Get(),
	}
}

// Main runs portcheck with args (excluding the program name) against the real
// OS and returns the process exit code. Interrupts cancel the command.
func Main(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Execute(ctx, NewApp(), args)
}

// Execute runs the command line in args and returns the exit code. Errors are
// reported on app.Stderr.
func Execute(ctx context.Context, app *App, args []string) int {
	root := NewRootCmd(app)
	root.SetArgs(args)
	cmd, err := root.ExecuteContextC(ctx)
	code := exitCode(err)
	if err != nil && !isSilent(err) {
		fmt.Fprintf(app.Stderr, "portcheck: %v\n", err)
		if code == ExitUsage {
			fmt.Fprintf(app.Stderr, "Run '%s --help' for usage.\n", cmd.CommandPath())
		}
	}
	return code
}
