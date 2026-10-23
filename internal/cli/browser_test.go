package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/roystondwork-dev/portcheck/internal/nativehost"
	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

// browserHarness is a harness whose home directory has Chrome "installed".
func browserHarness(t *testing.T) (*harness, string) {
	t.Helper()
	h := newHarness()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config", "google-chrome"), 0o750); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(home, "bin", "portcheck")
	if err := os.MkdirAll(filepath.Dir(exe), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	h.app.BrowserEnv = func() (nativehost.Env, error) { return nativehost.Env{GOOS: "linux", Home: home}, nil }
	h.app.Executable = func() (string, error) { return exe, nil }
	h.app.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	return h, home
}

func browserStatus(t *testing.T, h *harness) output.BrowsersJSON {
	t.Helper()
	h.stdout.Reset()
	if code := h.run(t, "browser", "status", "--json"); code != ExitOK {
		t.Fatalf("status exit %d: %s", code, h.stderr)
	}
	var env struct{ Data output.BrowsersJSON }
	if err := json.Unmarshal(h.stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

func TestBrowserInstallStatusUninstall(t *testing.T) {
	t.Parallel()
	h, home := browserHarness(t)
	if code := h.run(t, "browser", "install"); code != ExitOK {
		t.Fatalf("install exit %d: %s", code, h.stderr)
	}
	if !strings.Contains(h.stderr.String(), "Registered with Google Chrome.") || strings.Contains(h.stderr.String(), "Edge") {
		t.Errorf("stderr = %s", h.stderr)
	}
	st := browserStatus(t, h)
	chrome, edge := st.Browsers[0], st.Browsers[1]
	if st.Host != nativehost.HostName || !chrome.Registered || !chrome.Detected || edge.Registered || edge.Detected {
		t.Errorf("status = %+v", st)
	}
	if chrome.Path != filepath.Join(home, "bin", "portcheck") || !slices.Equal(chrome.Extensions, nativehost.ExtensionIDs) {
		t.Errorf("chrome = %+v", chrome)
	}

	if code := h.run(t, "browser", "uninstall", "--browser", "chrome"); code != ExitOK {
		t.Fatalf("uninstall exit %d: %s", code, h.stderr)
	}
	if browserStatus(t, h).Browsers[0].Registered {
		t.Error("still registered after uninstall")
	}
	if code := h.run(t, "browser", "uninstall"); code != ExitOK {
		t.Errorf("uninstall all: exit %d: %s", code, h.stderr)
	}
}

func TestBrowserInstallOptions(t *testing.T) {
	t.Parallel()
	h, _ := browserHarness(t)
	// Register with a browser that isn't detected, a development extension ID and an explicit path.
	code := h.run(t, "browser", "install", "--browser", "edge", "--extension-id", "aaaabbbbccccddddeeeeffffgggghhhh", "--path", "relative/portcheck", "-o", "plain")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	edge := browserStatus(t, h).Browsers[1]
	wd, _ := os.Getwd()
	if !edge.Registered || edge.Path != filepath.Join(wd, "relative", "portcheck") || len(edge.Extensions) != len(nativehost.ExtensionIDs)+1 {
		t.Errorf("edge = %+v", edge)
	}
}

func TestBrowserInstallPrefersPath(t *testing.T) {
	t.Parallel()
	h, home := browserHarness(t)
	link := filepath.Join(home, "path-bin", "portcheck")
	if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "bin", "portcheck"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	h.app.LookPath = func(string) (string, error) { return link, nil }
	if code := h.run(t, "browser", "install"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if got := browserStatus(t, h).Browsers[0].Path; got != link {
		t.Errorf("path = %s, want the PATH copy %s", got, link)
	}
}

func TestBrowserErrors(t *testing.T) {
	t.Parallel()
	envErr := errors.New("no home")
	cases := []runCase{
		{name: "unknown browser", args: []string{"browser", "install", "--browser", "opera"}, wantCode: ExitUsage, wantErr: []string{`unknown browser "opera"`}},
		{name: "unknown browser uninstall", args: []string{"browser", "uninstall", "--browser", "safari"}, wantCode: ExitUsage, wantErr: []string{`unknown browser "safari"`}},
		{name: "bad extension id", args: []string{"browser", "install", "--extension-id", "xyz"}, wantCode: ExitUsage, wantErr: []string{`invalid extension ID "xyz"`}},
		{name: "bad extension id letters", args: []string{"browser", "install", "--extension-id", "zzzzbbbbccccddddeeeeffffgggghhhh"}, wantCode: ExitUsage, wantErr: []string{"invalid extension ID"}},
		{name: "no args", args: []string{"browser", "status", "extra"}, wantCode: ExitUsage},
		{
			name: "no browser found", args: []string{"browser", "install"}, wantCode: ExitRuntime, wantErr: []string{"found no supported browser"},
			setup: func(h *harness) {
				home := filepath.Join(os.TempDir(), "portcheck-nobrowser-home")
				h.app.BrowserEnv = func() (nativehost.Env, error) { return nativehost.Env{GOOS: "linux", Home: home}, nil }
			},
		},
		{
			name: "env error", args: []string{"browser", "status"}, wantCode: ExitRuntime, wantErr: []string{"no home"},
			setup: func(h *harness) {
				h.app.BrowserEnv = func() (nativehost.Env, error) { return nativehost.Env{}, envErr }
			},
		},
		{
			name: "unsupported os", args: []string{"browser", "install"}, wantCode: ExitRuntime, wantErr: []string{"isn't supported on plan9"},
			setup: func(h *harness) {
				h.app.BrowserEnv = func() (nativehost.Env, error) { return nativehost.Env{GOOS: "plan9", Home: "/h"}, nil }
			},
		},
		{
			name: "env error uninstall", args: []string{"browser", "uninstall"}, wantCode: ExitRuntime,
			setup: func(h *harness) {
				h.app.BrowserEnv = func() (nativehost.Env, error) { return nativehost.Env{}, envErr }
			},
		},
	}
	runCases(t, cases)
}

func TestBrowserFilesystemErrors(t *testing.T) {
	t.Parallel()
	h, _ := browserHarness(t)
	h.app.Executable = func() (string, error) { return "", errors.New("unknown") }
	if code := h.run(t, "browser", "install"); code != ExitRuntime || !strings.Contains(h.stderr.String(), "use --path") {
		t.Errorf("executable error: exit %d: %s", code, h.stderr)
	}

	// Chrome's NativeMessagingHosts is a file: installing fails.
	h, home := browserHarness(t)
	if err := os.WriteFile(filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if code := h.run(t, "browser", "install"); code != ExitRuntime {
		t.Errorf("install over a file: exit %d", code)
	}

	// A broken manifest makes status fail; a manifest that is a non-empty directory makes uninstall fail.
	h, home = browserHarness(t)
	hosts := filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts")
	if err := os.MkdirAll(hosts, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hosts, nativehost.HostName+".json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := h.run(t, "browser", "status"); code != ExitRuntime {
		t.Errorf("status with broken manifest: exit %d", code)
	}
	h, home = browserHarness(t)
	if err := os.MkdirAll(filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts", nativehost.HostName+".json", "x"), 0o750); err != nil {
		t.Fatal(err)
	}
	if code := h.run(t, "browser", "uninstall"); code != ExitRuntime {
		t.Errorf("uninstall a directory: exit %d", code)
	}
}

func TestValidExtensionID(t *testing.T) {
	t.Parallel()
	for id, want := range map[string]bool{
		nativehost.ExtensionIDs[0]:         true,
		"abcdefghijklmnopabcdefghijklmnop": true,
		"abcdefghijklmnopabcdefghijklmnoq": false,
		"ABCDEFGHIJKLMNOPABCDEFGHIJKLMNOP": false,
		"short":                            false,
	} {
		if got := validExtensionID(id); got != want {
			t.Errorf("validExtensionID(%q) = %v", id, got)
		}
	}
}

// nativeExchange sends requests to portcheck in host mode and returns the responses.
func nativeExchange(t *testing.T, h *harness, reqs ...string) ([]nativehost.Response, int) {
	t.Helper()
	var in bytes.Buffer
	for _, r := range reqs {
		if err := nativehost.WriteMessage(&in, []byte(r)); err != nil {
			t.Fatal(err)
		}
	}
	h.app.Stdin = &in
	code := h.run(t, "chrome-extension://"+nativehost.ExtensionIDs[0]+"/", "--parent-window=0")
	var out []nativehost.Response
	for {
		raw, err := nativehost.ReadMessage(h.stdout)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var r nativehost.Response
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out, code
}

func TestNativeHost(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.killer.checkErr = map[int32]error{1: errors.New("refusing to kill launchd (pid 1): protected system process")}
	h.finder.statuses = []freeport.Status{{Port: 3000, Proto: inspect.TCP, Availability: freeport.InUse, Holders: []inspect.Socket{{RawSocket: nodeSock, Process: &inspect.ProcessInfo{PID: 100, Name: "node"}}}}}
	resps, code := nativeExchange(t, h,
		`{"id":1,"op":"version"}`,
		`{"id":2,"op":"check","port":3000}`,
		`{"id":3,"op":"list"}`,
		`{"id":4,"op":"kill","pid":100}`,
		`{"id":5,"op":"kill","pid":1}`,
		`{"id":6,"op":"run"}`,
	)
	if code != ExitOK || len(resps) != 6 {
		t.Fatalf("exit %d, %d responses; stderr: %s", code, len(resps), h.stderr)
	}
	kind := func(r nativehost.Response) string {
		var env struct{ Kind string }
		_ = json.Unmarshal(r.Data, &env)
		return env.Kind
	}
	if r := resps[0]; !r.OK || kind(r) != "version" || !strings.Contains(string(r.Data), "v1.2.3") {
		t.Errorf("version = %+v", r)
	}
	if r := resps[1]; !r.OK || kind(r) != "check" || r.ExitCode != ExitNegative || !strings.Contains(string(r.Data), `"node"`) {
		t.Errorf("check = %+v", r)
	}
	if r := resps[2]; !r.OK || kind(r) != "list" || !strings.Contains(string(r.Data), `"java"`) {
		t.Errorf("list = %+v", r)
	}
	if r := resps[3]; !r.OK || kind(r) != "kill" || !strings.Contains(string(r.Data), `"terminated"`) {
		t.Errorf("kill = %+v", r)
	}
	if !slices.Contains(h.killer.killed, 100) {
		t.Error("pid 100 not killed")
	}
	if r := resps[4]; r.ExitCode == ExitOK || slices.Contains(h.killer.killed, 1) || !strings.Contains(string(r.Data), "refusing to kill launchd") {
		t.Errorf("kill pid 1 = %+v", r)
	}
	if r := resps[5]; r.OK || r.ExitCode != ExitUsage || !strings.Contains(r.Error, "unsupported operation") {
		t.Errorf("run = %+v", r)
	}
	if h.stderr.Len() != 0 {
		t.Errorf("host wrote to stderr: %s", h.stderr)
	}
}

func TestNativeHostBadInput(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.app.Stdin = bytes.NewReader([]byte{5, 0})
	if code := h.run(t, "chrome-extension://"+nativehost.ExtensionIDs[0]+"/"); code != ExitRuntime || !strings.Contains(h.stderr.String(), "native messaging") {
		t.Errorf("exit %d, stderr %s", code, h.stderr)
	}
}

func TestBrowserInstallFirefox(t *testing.T) {
	t.Parallel()
	h, home := browserHarness(t)
	if err := os.MkdirAll(filepath.Join(home, ".mozilla"), 0o750); err != nil {
		t.Fatal(err)
	}
	// A Firefox development build's ID, alongside a Chromium one.
	code := h.run(t, "browser", "install", "--extension-id", "dev@example.org", "--extension-id", "aaaabbbbccccddddeeeeffffgggghhhh")
	if code != ExitOK || !strings.Contains(h.stderr.String(), "Registered with Mozilla Firefox.") {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	data, err := os.ReadFile(filepath.Join(home, ".mozilla", "native-messaging-hosts", nativehost.HostName+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var m nativehost.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.AllowedExtensions, append(slices.Clone(nativehost.GeckoIDs), "dev@example.org")) || m.AllowedOrigins != nil {
		t.Errorf("firefox manifest = %s", data)
	}
	st := browserStatus(t, h)
	ff := st.Browsers[len(st.Browsers)-1]
	if ff.Browser != "firefox" || !ff.Registered || !slices.Equal(ff.Extensions, m.AllowedExtensions) {
		t.Errorf("firefox status = %+v", ff)
	}
	// The Chromium manifest got the Chromium ID, not the Firefox one.
	if c := st.Browsers[0]; !slices.Contains(c.Extensions, "aaaabbbbccccddddeeeeffffgggghhhh") || slices.Contains(c.Extensions, "dev@example.org") {
		t.Errorf("chrome status = %+v", c)
	}
}

func TestValidGeckoID(t *testing.T) {
	t.Parallel()
	for id, want := range map[string]bool{
		"portcheck@roystondwork-dev.github.io":   true,
		"@my-addon":                              true, // allowed by Firefox (MDN uses "@extension-without-data-collection")
		"{12345678-1234-1234-1234-123456789abc}": true,
		"{not-a-guid}":                           false,
		"no-at-sign":                             false,
		"a b@c.d":                                false,
	} {
		if got := validGeckoID(id); got != want {
			t.Errorf("validGeckoID(%q) = %v", id, got)
		}
	}
}

func TestNativeHostFirefoxLaunch(t *testing.T) {
	t.Parallel()
	h := newHarness()
	var in bytes.Buffer
	if err := nativehost.WriteMessage(&in, []byte(`{"id":1,"op":"version"}`)); err != nil {
		t.Fatal(err)
	}
	h.app.Stdin = &in
	manifest := filepath.Join("home", ".mozilla", "native-messaging-hosts", nativehost.HostName+".json")
	if code := h.run(t, manifest, nativehost.GeckoIDs[0]); code != ExitOK {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	raw, err := nativehost.ReadMessage(h.stdout)
	if err != nil || !strings.Contains(string(raw), `"kind":"version"`) {
		t.Errorf("response = %s, %v", raw, err)
	}
}
