//go:build !windows

package nativehost

import (
	"runtime"
	"testing"
)

func TestSystemEnv(t *testing.T) { //nolint:paralleltest // sets HOME
	t.Setenv("HOME", "/home/alice")
	env, err := SystemEnv()
	if err != nil || env.Home != "/home/alice" || env.GOOS != runtime.GOOS {
		t.Errorf("SystemEnv = %+v, %v", env, err)
	}
	t.Setenv("HOME", "")
	if _, err := SystemEnv(); err == nil {
		t.Error("no error without HOME")
	}
}
