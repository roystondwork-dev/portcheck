//go:build windows

package cli_test

func killSelf() int { return 1 } // no signals on Windows; not used there
