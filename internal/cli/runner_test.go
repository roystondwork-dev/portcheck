package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const childEnvVar = "PORTCHECK_TEST_CHILD" // handled by TestMain in script_test.go

func childSpec(mode string, args ...string) (RunSpec, *bytes.Buffer) {
	var out bytes.Buffer
	return RunSpec{
		Name:   os.Args[0],
		Args:   args,
		Env:    append(os.Environ(), childEnvVar+"="+mode, "PORT=4242"),
		Stdout: &out,
		Stderr: &out,
	}, &out
}

func realRunner() execRunner {
	return execRunner{notify: func(chan<- os.Signal, ...os.Signal) {}, stop: func(chan<- os.Signal) {}}
}

func TestRunnerExitCodeEnvAndArgs(t *testing.T) {
	t.Parallel()
	spec, out := childSpec("echo", "a", "b c")
	started := false
	code, err := realRunner().Run(spec, func() { started = true })
	if err != nil || code != 0 || !started {
		t.Fatalf("Run = %d, %v; started=%v", code, err, started)
	}
	if got := strings.TrimSpace(out.String()); got != "PORT=4242 ARGS=a,b c" {
		t.Errorf("child output = %q", got)
	}

	spec, _ = childSpec("exit:7")
	if code, err := realRunner().Run(spec, nil); code != 7 || err != nil {
		t.Errorf("exit:7 = %d, %v", code, err)
	}
}

func TestRunnerNotFound(t *testing.T) {
	t.Parallel()
	code, err := realRunner().Run(RunSpec{Name: "portcheck-definitely-missing-command"}, func() { t.Error("started called") })
	if code != exitNotFound || err == nil {
		t.Errorf("Run = %d, %v; want 127 and an error", code, err)
	}
}

func TestRunnerNotExecutable(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("execute permission bits don't exist on Windows")
	}
	f := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(f, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, err := realRunner().Run(RunSpec{Name: f}, nil); code != exitCannotExecute || err == nil {
		t.Errorf("Run = %d, %v; want 126 and an error", code, err)
	}
}

func TestRunnerKilledBySignal(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("no signals on Windows")
	}
	spec, _ := childSpec("sigterm")
	if code, err := realRunner().Run(spec, nil); code != 128+15 || err != nil {
		t.Errorf("Run = %d, %v; want 143", code, err)
	}
}

func TestRunnerRegistersForwarding(t *testing.T) {
	t.Parallel()
	var registered []os.Signal
	stopped := false
	r := execRunner{
		notify: func(_ chan<- os.Signal, sig ...os.Signal) { registered = sig },
		stop:   func(chan<- os.Signal) { stopped = true },
	}
	spec, _ := childSpec("exit:0")
	if _, err := r.Run(spec, nil); err != nil {
		t.Fatal(err)
	}
	if len(registered) != len(forwardedSignals) || !stopped {
		t.Errorf("registered %v (want %v), stopped=%v", registered, forwardedSignals, stopped)
	}
}

type recordingSignaler struct {
	mu   sync.Mutex
	sigs []os.Signal
}

func (r *recordingSignaler) Signal(s os.Signal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sigs = append(r.sigs, s)
	return errors.New("process already finished") // forwarding must ignore errors
}

func TestForward(t *testing.T) {
	t.Parallel()
	sigs := make(chan os.Signal, 2)
	done := make(chan struct{})
	rec := &recordingSignaler{}
	finished := make(chan struct{})
	go func() { forward(rec, sigs, done); close(finished) }()
	sigs <- os.Interrupt
	sigs <- os.Kill
	deadline := time.Now().Add(2 * time.Second)
	for {
		rec.mu.Lock()
		n := len(rec.sigs)
		rec.mu.Unlock()
		if n == 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(done)
	<-finished
	if len(rec.sigs) != 2 || rec.sigs[0] != os.Interrupt || rec.sigs[1] != os.Kill {
		t.Errorf("forwarded %v", rec.sigs)
	}
}

func TestStartFailureCode(t *testing.T) {
	t.Parallel()
	for err, want := range map[error]int{
		exec.ErrNotFound:          exitNotFound,
		os.ErrNotExist:            exitNotFound,
		syscall.EACCES:            exitCannotExecute,
		errors.New("exec format"): exitCannotExecute,
	} {
		if got := startFailureCode(err); got != want {
			t.Errorf("startFailureCode(%v) = %d, want %d", err, got, want)
		}
	}
}

func TestExitStatusWithoutProcessState(t *testing.T) {
	t.Parallel()
	boom := errors.New("wait failed")
	if code, err := exitStatus(nil, boom); code != ExitRuntime || !errors.Is(err, boom) {
		t.Errorf("exitStatus(nil) = %d, %v", code, err)
	}
}

func TestSignalExitCodeForNormalExit(t *testing.T) {
	t.Parallel()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), childEnvVar+"=exit:0")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if got := signalExitCode(cmd.ProcessState); got != ExitRuntime {
		t.Errorf("signalExitCode(normal exit) = %d", got)
	}
}
