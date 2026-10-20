package nativehost

import (
	"testing"
)

func TestSystemEnv(t *testing.T) { //nolint:paralleltest // sets LOCALAPPDATA
	t.Setenv("LOCALAPPDATA", `C:\Users\alice\AppData\Local`)
	env, err := SystemEnv()
	if err != nil || env.DataDir != `C:\Users\alice\AppData\Local\portcheck` || env.GOOS != "windows" || env.Registry == nil {
		t.Errorf("SystemEnv = %+v, %v", env, err)
	}
	t.Setenv("LOCALAPPDATA", "")
	if _, err := SystemEnv(); err == nil {
		t.Error("no error without LOCALAPPDATA")
	}
}
