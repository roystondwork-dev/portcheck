package buildinfo

import (
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// setVars swaps the package-level build variables for the duration of a test.
// Tests using it must not run in parallel.
func setVars(t *testing.T, v, c, d string, rbi func() (*debug.BuildInfo, bool)) {
	t.Helper()
	oldV, oldC, oldD, oldR := version, commit, date, readBuildInfo
	version, commit, date, readBuildInfo = v, c, d, rbi
	t.Cleanup(func() { version, commit, date, readBuildInfo = oldV, oldC, oldD, oldR })
}

func noBuildInfo() (*debug.BuildInfo, bool) { return nil, false }

func TestGet(t *testing.T) {
	platform := runtime.GOOS + "/" + runtime.GOARCH

	tests := []struct {
		name                  string
		version, commit, date string
		bi                    *debug.BuildInfo
		want                  Info
	}{
		{
			name:    "ldflags only, no build info",
			version: "v1.2.3", commit: "abc", date: "2026-01-01",
			want: Info{Version: "v1.2.3", Commit: "abc", Date: "2026-01-01"},
		},
		{
			name:    "falls back to module version and vcs settings",
			version: "dev",
			bi: &debug.BuildInfo{
				Main: debug.Module{Version: "v0.4.0"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "deadbeef"},
					{Key: "vcs.time", Value: "2026-10-20T00:00:00Z"},
					{Key: "vcs.modified", Value: "true"},
					{Key: "GOOS", Value: "ignored"},
				},
			},
			want: Info{Version: "v0.4.0", Commit: "deadbeef", Date: "2026-10-20T00:00:00Z", Modified: true},
		},
		{
			name:    "devel module version is ignored",
			version: "dev",
			bi:      &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			want:    Info{Version: "dev"},
		},
		{
			name:    "ldflags take precedence over build info",
			version: "v9.9.9", commit: "fromldflags", date: "ldate",
			bi: &debug.BuildInfo{
				Main: debug.Module{Version: "v0.0.1"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "fromvcs"},
					{Key: "vcs.time", Value: "vtime"},
					{Key: "vcs.modified", Value: "false"},
				},
			},
			want: Info{Version: "v9.9.9", Commit: "fromldflags", Date: "ldate"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbi := noBuildInfo
			if tt.bi != nil {
				rbi = func() (*debug.BuildInfo, bool) { return tt.bi, true }
			}
			setVars(t, tt.version, tt.commit, tt.date, rbi)

			tt.want.GoVersion = runtime.Version()
			tt.want.Platform = platform
			if diff := cmp.Diff(tt.want, Get()); diff != "" {
				t.Errorf("Get() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInfoString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		info Info
		want string
	}{
		{
			name: "full",
			info: Info{Version: "v1.0.0", Commit: "0123456789abcdef", Date: "2026-10-20", GoVersion: "go1.27", Platform: "darwin/arm64"},
			want: "portcheck v1.0.0 (commit 0123456789ab, built 2026-10-20, go1.27, darwin/arm64)",
		},
		{
			name: "unknown commit and date, dirty",
			info: Info{Version: "dev", Modified: true, GoVersion: "go1.27", Platform: "linux/amd64"},
			want: "portcheck dev (commit unknown-dirty, built unknown, go1.27, linux/amd64)",
		},
		{
			name: "short commit kept as is",
			info: Info{Version: "dev", Commit: "abc", Date: "d", GoVersion: "g", Platform: "p"},
			want: "portcheck dev (commit abc, built d, g, p)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.info.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultReadBuildInfo(t *testing.T) {
	// The real implementation must be wired in by default; in tests the
	// toolchain always embeds build info.
	if _, ok := readBuildInfo(); !ok {
		t.Fatal("expected debug.ReadBuildInfo to report build info in tests")
	}
}
