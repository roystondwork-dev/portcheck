package nativehost

import (
	"errors"
	"os"
	"path/filepath"
)

// SystemEnv returns the registration environment of this machine.
func SystemEnv() (Env, error) {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		return Env{}, errors.New("LOCALAPPDATA is not set")
	}
	return Env{GOOS: "windows", DataDir: filepath.Join(dir, "portcheck"), Registry: SystemRegistry{}}, nil
}
