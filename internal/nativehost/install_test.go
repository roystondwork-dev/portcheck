package nativehost

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// fakeRegistry is an in-memory HKEY_CURRENT_USER.
type fakeRegistry struct {
	keys       map[string]string // key -> default value ("" if none)
	failSet    bool
	failGet    bool
	failDelete bool
}

func newFakeRegistry(keys ...string) *fakeRegistry {
	r := &fakeRegistry{keys: map[string]string{}}
	for _, k := range keys {
		r.keys[k] = ""
	}
	return r
}

func (r *fakeRegistry) KeyExists(key string) bool { _, ok := r.keys[key]; return ok }

func (r *fakeRegistry) SetDefault(key, value string) error {
	if r.failSet {
		return errors.New("access denied")
	}
	r.keys[key] = value
	return nil
}

func (r *fakeRegistry) DefaultValue(key string) (string, error) {
	if r.failGet {
		return "", errors.New("access denied")
	}
	return r.keys[key], nil
}

func (r *fakeRegistry) DeleteKey(key string) error {
	if r.failDelete {
		return errors.New("access denied")
	}
	delete(r.keys, key)
	return nil
}

func TestTargetsUnix(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config", "microsoft-edge"), 0o750); err != nil {
		t.Fatal(err)
	}
	targets, err := Targets(Env{GOOS: "linux", Home: home})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(targets))
	for _, tg := range targets {
		rel, _ := filepath.Rel(home, tg.Manifest)
		got = append(got, tg.Browser.ID+" "+filepath.ToSlash(rel)+" "+map[bool]string{true: "detected", false: "-"}[tg.Detected])
	}
	want := []string{
		"chrome .config/google-chrome/NativeMessagingHosts/io.github.roystondwork_dev.portcheck.json -",
		"edge .config/microsoft-edge/NativeMessagingHosts/io.github.roystondwork_dev.portcheck.json detected",
		"chromium .config/chromium/NativeMessagingHosts/io.github.roystondwork_dev.portcheck.json -",
		"brave .config/BraveSoftware/Brave-Browser/NativeMessagingHosts/io.github.roystondwork_dev.portcheck.json -",
	}
	if !cmp.Equal(got, want) {
		t.Error(cmp.Diff(want, got))
	}

	mac, err := Targets(Env{GOOS: "darwin", Home: home})
	if err != nil || !strings.HasSuffix(filepath.ToSlash(mac[0].Manifest), "Library/Application Support/Google/Chrome/NativeMessagingHosts/"+HostName+".json") {
		t.Errorf("darwin chrome = %+v, %v", mac[0], err)
	}
}

func TestTargetsWindows(t *testing.T) {
	t.Parallel()
	reg := newFakeRegistry(`Software\Google\Chrome`)
	targets, err := Targets(Env{GOOS: "windows", DataDir: `C:\Users\alice\AppData\Local\portcheck`, Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	c := targets[0]
	if c.RegistryKey != `Software\Google\Chrome\NativeMessagingHosts\`+HostName || !c.Detected || targets[1].Detected {
		t.Errorf("targets = %+v", targets)
	}
	if !strings.Contains(c.Manifest, "native-messaging") || !strings.HasSuffix(c.Manifest, HostName+".json") {
		t.Errorf("manifest path = %s", c.Manifest)
	}
}

func TestTargetsErrors(t *testing.T) {
	t.Parallel()
	for name, env := range map[string]Env{
		"unsupported OS":       {GOOS: "plan9", Home: "/home/a"},
		"no home":              {GOOS: "linux"},
		"windows without data": {GOOS: "windows", Registry: newFakeRegistry()},
		"windows without reg":  {GOOS: "windows", DataDir: `C:\x`},
	} {
		if _, err := Targets(env); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestSelect(t *testing.T) {
	t.Parallel()
	targets := []Target{{Browser: Browsers[0], Detected: true}, {Browser: Browsers[1]}, {Browser: Browsers[2], Detected: true}}
	ids := func(ts []Target) (out []string) {
		for _, t := range ts {
			out = append(out, t.Browser.ID)
		}
		return out
	}
	if got, _ := Select(targets, nil); !cmp.Equal(ids(got), []string{"chrome", "chromium"}) {
		t.Errorf("default = %v", ids(got))
	}
	if got, _ := Select(targets, []string{"edge"}); !cmp.Equal(ids(got), []string{"edge"}) {
		t.Errorf("edge = %v", ids(got))
	}
	if got, _ := Select(targets[1:2], nil); got != nil {
		t.Errorf("none detected = %v", got)
	}
	if _, err := Select(targets, []string{"firefox"}); err == nil || !strings.Contains(err.Error(), "unknown browser") {
		t.Errorf("firefox: %v", err)
	}
}

func TestInstallUnix(t *testing.T) {
	t.Parallel()
	env := Env{GOOS: "linux", Home: t.TempDir()}
	targets, _ := Targets(env)
	tg := targets[0]
	if m, err := Installed(env, tg); m != nil || err != nil {
		t.Fatalf("before install: %v, %v", m, err)
	}
	m := NewManifest("/usr/local/bin/portcheck", []string{"aaaabbbbccccddddeeeeffffgggghhhh", "pppppppppppppppppppppppppppppppp"})
	if err := Install(env, tg, m); err != nil {
		t.Fatal(err)
	}
	got, err := Installed(env, tg)
	if err != nil {
		t.Fatal(err)
	}
	want := Manifest{
		Name: HostName, Path: "/usr/local/bin/portcheck", Type: "stdio", Description: m.Description,
		AllowedOrigins: []string{"chrome-extension://aaaabbbbccccddddeeeeffffgggghhhh/", "chrome-extension://pppppppppppppppppppppppppppppppp/"},
	}
	if !cmp.Equal(*got, want) {
		t.Error(cmp.Diff(want, *got))
	}
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(tg.Manifest); st.Mode().Perm() != 0o600 {
			t.Errorf("manifest mode = %v", st.Mode())
		}
	}
	if err := Uninstall(env, tg); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(env, tg); err != nil {
		t.Fatalf("second uninstall: %v", err)
	}
	if m, _ := Installed(env, tg); m != nil {
		t.Error("still installed")
	}
}

func TestInstallWindows(t *testing.T) {
	t.Parallel()
	reg := newFakeRegistry()
	env := Env{GOOS: "windows", DataDir: t.TempDir(), Registry: reg}
	targets, _ := Targets(env)
	tg := targets[1] // edge
	if m, err := Installed(env, tg); m != nil || err != nil {
		t.Fatalf("before install: %v, %v", m, err)
	}
	if err := Install(env, tg, NewManifest(`C:\bin\portcheck.exe`, ExtensionIDs)); err != nil {
		t.Fatal(err)
	}
	if reg.keys[tg.RegistryKey] != tg.Manifest {
		t.Errorf("registry = %v", reg.keys)
	}
	if m, err := Installed(env, tg); err != nil || m.Path != `C:\bin\portcheck.exe` {
		t.Errorf("installed = %+v, %v", m, err)
	}
	if err := Uninstall(env, tg); err != nil || reg.KeyExists(tg.RegistryKey) {
		t.Errorf("uninstall: %v, keys %v", err, reg.keys)
	}
}

func TestInstallErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m := NewManifest("/bin/portcheck", ExtensionIDs)
	b := Browsers[0]

	// The manifest's directory can't be created, or the manifest can't be written.
	if err := Install(Env{}, Target{Browser: b, Manifest: filepath.Join(file, "sub", "m.json")}, m); err == nil {
		t.Error("mkdir under a file succeeded")
	}
	if err := Install(Env{}, Target{Browser: b, Manifest: dir}, m); err == nil {
		t.Error("writing over a directory succeeded")
	}
	// The registry refuses.
	reg := newFakeRegistry()
	reg.failSet = true
	err := Install(Env{Registry: reg}, Target{Browser: b, Manifest: filepath.Join(dir, "m.json"), RegistryKey: `k`}, m)
	if err == nil || !strings.Contains(err.Error(), "register with Google Chrome") {
		t.Errorf("registry failure: %v", err)
	}

	reg.failDelete = true
	if err := Uninstall(Env{Registry: reg}, Target{Browser: b, Manifest: "x", RegistryKey: "k"}); err == nil {
		t.Error("registry delete failure ignored")
	}
	// A non-empty directory where the manifest should be can't be removed.
	full := filepath.Join(dir, "full")
	if err := os.MkdirAll(filepath.Join(full, "child"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(Env{}, Target{Browser: b, Manifest: full}); err == nil {
		t.Error("removing a non-empty directory succeeded")
	}
}

func TestInstalledErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	b := Browsers[0]
	if _, err := Installed(Env{}, Target{Browser: b, Manifest: bad}); err == nil {
		t.Error("invalid JSON accepted")
	}
	if _, err := Installed(Env{}, Target{Browser: b, Manifest: dir}); err == nil {
		t.Error("reading a directory succeeded")
	}
	reg := newFakeRegistry("k")
	reg.failGet = true
	if _, err := Installed(Env{Registry: reg}, Target{Browser: b, RegistryKey: "k"}); err == nil {
		t.Error("registry read failure ignored")
	}
}

func TestHostPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	exe := filepath.Join(dir, "portcheck")
	other := filepath.Join(dir, "other")
	for _, f := range []string{exe, other} {
		if err := os.WriteFile(f, []byte(f), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := HostPath(exe, ""); got != exe {
		t.Errorf("no PATH copy: %s", got)
	}
	if got := HostPath(exe, other); got != exe {
		t.Errorf("different file on PATH: %s", got)
	}
	if got := HostPath(exe, filepath.Join(dir, "missing")); got != exe {
		t.Errorf("missing PATH copy: %s", got)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(exe, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got := HostPath(exe, link); got != link {
		t.Errorf("PATH symlink to this binary: got %s, want %s", got, link)
	}
}
