//go:build !windows

package inspect

import "os"

// geteuid is replaced in tests.
var geteuid = os.Geteuid

func isElevated() bool { return geteuid() == 0 }
