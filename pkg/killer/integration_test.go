//go:build integration

package killer

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// The test binary re-executes itself as a helper process when this variable
// is set, giving us a real, killable child on every OS.
const helperEnv = "PORTCHECK_KILLER_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "sleep":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM)
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// startHelper starts a helper child and reaps it in the background so that it
// does not linger as a zombie once killed.
func startHelper(t *testing.T, mode string) int32 {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), helperEnv+"="+mode)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	// Give the helper time to install its signal handler.
	time.Sleep(300 * time.Millisecond)
	return int32(cmd.Process.Pid)
}

func TestIntegrationGracefulTerminate(t *testing.T) {
	pid := startHelper(t, "sleep")
	res, err := New().Kill(t.Context(), pid, Options{Grace: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	want := Terminated
	if res.Outcome != want {
		t.Errorf("outcome = %v, want %v", res.Outcome, want)
	}
	if res.Name == "" {
		t.Error("process name not resolved")
	}
}

func TestIntegrationForceAfterGrace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no ignorable graceful signal; TerminateProcess always succeeds")
	}
	pid := startHelper(t, "ignore-term")
	res, err := New().Kill(t.Context(), pid, Options{Grace: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != Killed {
		t.Errorf("outcome = %v, want killed", res.Outcome)
	}
}

func TestIntegrationForce(t *testing.T) {
	pid := startHelper(t, "sleep")
	res, err := New().Kill(t.Context(), pid, Options{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != Killed {
		t.Errorf("outcome = %v, want killed", res.Outcome)
	}
}

func TestIntegrationNotFound(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	ops := systemOps{}
	pid := int32(cmd.Process.Pid) // exited and reaped
	if _, err := ops.Name(t.Context(), pid); !errors.Is(err, ErrNotFound) {
		t.Errorf("Name(exited) = %v, want ErrNotFound", err)
	}
	if err := ops.Terminate(t.Context(), pid); !errors.Is(err, ErrNotFound) {
		t.Errorf("Terminate(exited) = %v, want ErrNotFound", err)
	}
	if err := ops.Kill(t.Context(), pid); !errors.Is(err, ErrNotFound) {
		t.Errorf("Kill(exited) = %v, want ErrNotFound", err)
	}
	if ops.Alive(t.Context(), pid) {
		t.Error("Alive(exited) = true")
	}
}

func TestIntegrationZombieIsNotAlive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no zombies on Windows")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := int32(cmd.Process.Pid)
	// Wait for the child to exit without reaping it.
	ops := systemOps{}
	deadline := time.Now().Add(5 * time.Second)
	for ops.Alive(t.Context(), pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if ops.Alive(t.Context(), pid) {
		t.Error("exited, unreaped child reported alive")
	}
	_ = cmd.Wait()
}
