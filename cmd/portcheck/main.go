// Command portcheck shows what is using your ports, finds free ones, frees
// them up, and checks ports on remote hosts.
//
// See https://github.com/roystondwork-dev/portcheck for documentation.
package main

import (
	"os"

	"github.com/roystondwork-dev/portcheck/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
