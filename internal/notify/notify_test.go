package notify

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

type call struct {
	env  []string
	name string
	args []string
}

func recorder(err error) (Run, *[]call) {
	var calls []call
	return func(_ context.Context, env []string, name string, args ...string) error {
		calls = append(calls, call{env, name, args})
		return err
	}, &calls
}

func TestNotify(t *testing.T) {
	t.Parallel()
	title, msg := `portcheck "x"`, `site is down: it's "gone" $(rm -rf ~)`
	for goos, check := range map[string]func(t *testing.T, c call){
		"darwin": func(t *testing.T, c call) {
			t.Helper()
			// Text is passed as arguments, after the script.
			if c.name != "osascript" || c.args[len(c.args)-2] != title || c.args[len(c.args)-1] != msg || strings.Contains(c.args[1], "rm -rf") {
				t.Errorf("call = %+v", c)
			}
		},
		"windows": func(t *testing.T, c call) {
			t.Helper()
			// Text goes in the environment; the script is fixed.
			if c.name != "powershell" || !slices.Contains(c.env, "PORTCHECK_NOTIFY_MESSAGE="+msg) ||
				!slices.Contains(c.env, "PORTCHECK_NOTIFY_TITLE="+title) || c.args[len(c.args)-1] != toastScript {
				t.Errorf("call = %+v", c)
			}
		},
		"linux": func(t *testing.T, c call) {
			t.Helper()
			if c.name != "notify-send" || !slices.Equal(c.args, []string{"--app-name=portcheck", title, msg}) {
				t.Errorf("call = %+v", c)
			}
		},
	} {
		run, calls := recorder(nil)
		if err := System(goos, run).Notify(t.Context(), title, msg); err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		if len(*calls) != 1 {
			t.Fatalf("%s: %d calls", goos, len(*calls))
		}
		check(t, (*calls)[0])
	}
}

func TestNotifyErrors(t *testing.T) {
	t.Parallel()
	run, calls := recorder(nil)
	if err := System("plan9", run).Notify(t.Context(), "t", "m"); !errors.Is(err, ErrUnsupported) || len(*calls) != 0 {
		t.Errorf("plan9: %v, %d calls", err, len(*calls))
	}
	boom := errors.New("notify-send: not found")
	run, _ = recorder(boom)
	if err := System("linux", run).Notify(t.Context(), "t", "m"); !errors.Is(err, boom) {
		t.Errorf("failing tool: %v", err)
	}
}

func TestExec(t *testing.T) {
	t.Parallel()
	if err := Exec(t.Context(), []string{"PORTCHECK_NOTIFY_TEST=1"}, "go", "env", "GOOS"); err != nil {
		t.Errorf("Exec(go env) = %v", err)
	}
	if err := Exec(t.Context(), nil, "go", "no-such-go-command"); err == nil || !strings.Contains(err.Error(), "go: exit status") {
		t.Errorf("Exec(failing) = %v", err)
	}
	if err := Exec(t.Context(), nil, "portcheck-no-such-program"); err == nil || !strings.Contains(err.Error(), "portcheck-no-such-program:") {
		t.Errorf("Exec(missing) = %v", err)
	}
}
