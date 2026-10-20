package cli

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
)

// RunSpec describes a command for ProcessRunner.
type RunSpec struct {
	Name   string
	Args   []string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// ProcessRunner runs a command to completion. started, if non-nil, is called
// once the command has been started. The returned code follows shell
// conventions: the command's exit status, 128+N if it was killed by signal N,
// 127 if it was not found and 126 if it could not be executed.
type ProcessRunner interface {
	Run(spec RunSpec, started func()) (code int, err error)
}

// Exit codes for commands that could not be started (shell conventions).
const (
	exitCannotExecute = 126
	exitNotFound      = 127
)

// signaler is the part of *os.Process used to forward signals.
type signaler interface {
	Signal(os.Signal) error
}

// execRunner runs real processes. notify/stop default to signal.Notify/Stop.
type execRunner struct {
	notify func(c chan<- os.Signal, sig ...os.Signal)
	stop   func(c chan<- os.Signal)
}

func (r execRunner) Run(spec RunSpec, started func()) (int, error) {
	// Not CommandContext: Ctrl+C cancels portcheck's context, and the child must
	// receive Ctrl+C itself and shut down gracefully rather than be killed.
	cmd := exec.Command(spec.Name, spec.Args...) //nolint:gosec,noctx // running the user's command is the purpose of `portcheck run`
	cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = spec.Env, spec.Stdin, spec.Stdout, spec.Stderr
	if err := cmd.Start(); err != nil {
		return startFailureCode(err), err
	}
	if started != nil {
		started()
	}

	sigs := make(chan os.Signal, 4)
	r.notify(sigs, forwardedSignals...)
	defer r.stop(sigs)
	done := make(chan struct{})
	forwarded := make(chan struct{})
	go func() {
		defer close(forwarded)
		forward(cmd.Process, sigs, done)
	}()

	err := cmd.Wait()
	close(done)
	<-forwarded
	return exitStatus(cmd.ProcessState, err)
}

// forward relays signals to p until done is closed.
func forward(p signaler, sigs <-chan os.Signal, done <-chan struct{}) {
	for {
		select {
		case s := <-sigs:
			_ = p.Signal(s) // the child may already have exited
		case <-done:
			return
		}
	}
}

func startFailureCode(err error) int {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return exitNotFound
	}
	return exitCannotExecute
}

// exitStatus converts a finished process into a shell-style exit code.
func exitStatus(ps *os.ProcessState, waitErr error) (int, error) {
	if ps == nil {
		return ExitRuntime, waitErr
	}
	if code := ps.ExitCode(); code >= 0 {
		return code, nil
	}
	return signalExitCode(ps), nil
}
