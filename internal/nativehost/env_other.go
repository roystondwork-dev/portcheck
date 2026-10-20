//go:build !windows

package nativehost

import (
	"fmt"
	"os"
	"runtime"
)

// SystemEnv returns the registration environment of this machine.
func SystemEnv() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, fmt.Errorf("find the home directory: %w", err)
	}
	return Env{GOOS: runtime.GOOS, Home: home}, nil
}
