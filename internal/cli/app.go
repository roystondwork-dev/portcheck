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
	"net"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/roystondwork-dev/portcheck/internal/buildinfo"
	"github.com/roystondwork-dev/portcheck/internal/tui"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
	"github.com/roystondwork-dev/portcheck/pkg/scan"
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

// ScanFunc runs a port scan; scan.Scan implements it.
type ScanFunc func(ctx context.Context, targets []scan.Target, ports portspec.Spec, opts scan.Options) <-chan scan.Result

// TUIFunc runs the interactive UI; tui.Run implements it.
type TUIFunc func(ctx context.Context, deps tui.Deps, in io.Reader, out io.Writer, environ []string) error

// App bundles everything a command needs from the outside world.
type App struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Environ        []string
	// StdinTTY, StdoutTTY and StderrTTY report whether the streams are terminals.
	StdinTTY, StdoutTTY, StderrTTY bool

	Lister        SocketLister
	Ports         PortFinder
	Killer        ProcessKiller
	PrivilegeHint func() string
	Resolver      scan.Resolver
	Scan          ScanFunc
	RunTUI        TUIFunc
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
		StderrTTY:     term.IsTerminal(int(os.Stderr.Fd())),
		Lister:        lister,
		Ports:         freeport.New(freeport.WithLister(lister)),
		Killer:        killer.New(),
		PrivilegeHint: inspect.PrivilegeHint,
		Resolver:      net.DefaultResolver,
		Scan:          scan.Scan,
		RunTUI:        tui.Run,
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
