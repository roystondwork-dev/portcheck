//go:build windows

package inspect

import "golang.org/x/sys/windows"

// tokenElevated is replaced in tests.
var tokenElevated = func() bool { return windows.GetCurrentProcessToken().IsElevated() }

func isElevated() bool { return tokenElevated() }
