package cli_test

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/roystondwork-dev/portcheck/internal/cli"
)

// holdPortEnv makes the test binary act as a helper that listens on a random
// TCP port, writes the port number to the named file, and waits to be killed.
const holdPortEnv = "PORTCHECK_TEST_HOLDPORT"

func TestMain(m *testing.M) {
	if file := os.Getenv(holdPortEnv); file != "" {
		holdPort(file)
		return
	}
	testscript.Main(m, map[string]func(){
		"portcheck": func() { os.Exit(cli.Main(os.Args[1:])) },
	})
}

func holdPort(file string) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	if err := os.WriteFile(file+".tmp", []byte(port), 0o600); err != nil {
		os.Exit(1)
	}
	if err := os.Rename(file+".tmp", file); err != nil {
		os.Exit(1)
	}
	time.Sleep(2 * time.Minute)
	os.Exit(0)
}

// TestScripts runs the end-to-end scenarios in testdata/script against the
// real portcheck command (re-executed test binary) and real sockets.
func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                 "testdata/script",
		RequireExplicitExec: true,
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"holdport": cmdHoldPort,
			"exits":    cmdExits,
		},
	})
}

// cmdHoldPort starts a helper process holding a TCP port and sets $PORT and
// $HOLDER_PID for the rest of the script.
func cmdHoldPort(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 0 {
		ts.Fatalf("usage: holdport")
	}
	file := filepath.Join(ts.Getenv("WORK"), "held-port")
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), holdPortEnv+"="+file)
	ts.Check(cmd.Start())
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	ts.Defer(func() {
		_ = cmd.Process.Kill()
		<-done
	})

	deadline := time.Now().Add(10 * time.Second)
	for {
		data, err := os.ReadFile(file)
		if err == nil {
			ts.Setenv("PORT", strings.TrimSpace(string(data)))
			ts.Setenv("HOLDER_PID", strconv.Itoa(cmd.Process.Pid))
			return
		}
		if time.Now().After(deadline) {
			ts.Fatalf("helper did not report its port: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// cmdExits runs a command and asserts its exact exit code:
//
//	exits 2 portcheck kill 3000
func cmdExits(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 2 {
		ts.Fatalf("usage: exits <code> <command> [args...]")
	}
	want, err := strconv.Atoi(args[0])
	ts.Check(err)
	err = ts.Exec(args[1], args[2:]...)
	got := 0
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		got = ee.ExitCode()
	case err != nil:
		ts.Fatalf("run %s: %v", args[1], err)
	}
	if got != want {
		ts.Fatalf("%s exited with %d, want %d", strings.Join(args[1:], " "), got, want)
	}
}
