// Package buildinfo reports the version, commit and build date of the running
// portcheck binary.
//
// Release builds inject values with -ldflags (see Makefile and .goreleaser.yaml).
// For `go install` builds, values fall back to the module and VCS information
// embedded by the Go toolchain.
package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Set via -ldflags "-X .../internal/buildinfo.version=..." at release time.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// readBuildInfo is replaced in tests.
var readBuildInfo = debug.ReadBuildInfo

// Info describes a portcheck build.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	Modified  bool   `json:"modified"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// Get returns information about the current build.
func Get() Info {
	info := Info{
		Version:   version,
		Commit:    commit,
		Date:      date,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}

	bi, ok := readBuildInfo()
	if !ok {
		return info
	}
	if info.Version == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		info.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if info.Commit == "" {
				info.Commit = s.Value
			}
		case "vcs.time":
			if info.Date == "" {
				info.Date = s.Value
			}
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	return info
}

// String renders a one-line human-readable summary.
func (i Info) String() string {
	commit := i.Commit
	if len(commit) > 12 {
		commit = commit[:12]
	}
	if commit == "" {
		commit = "unknown"
	}
	if i.Modified {
		commit += "-dirty"
	}
	date := i.Date
	if date == "" {
		date = "unknown"
	}
	return fmt.Sprintf("portcheck %s (commit %s, built %s, %s, %s)",
		i.Version, commit, date, i.GoVersion, i.Platform)
}
