package nativehost

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// ExtensionIDs are the portcheck extension IDs allowed to start the host by
// default: the ID pinned by the manifest key in extensions/browser (release
// zips, loaded unpacked), then the store builds' IDs.
var ExtensionIDs = []string{
	"fmocabgaaihiflefcdafnhoihcibgnhl", // release zip
	"jenlahoamefdpfmpodhidkgcceikheee", // Microsoft Edge Add-ons
}

// Browser is a Chromium browser portcheck can register with.
type Browser struct {
	// ID is used on the command line: chrome, chromium, edge, brave.
	ID   string
	Name string
	// Dir is the user data directory per OS (relative to the home directory on
	// macOS and Linux); the host manifest goes in its NativeMessagingHosts.
	Dir map[string]string
	// RegistryKey is the per-user registry key (under HKEY_CURRENT_USER) on
	// Windows whose NativeMessagingHosts subkey points to the manifest.
	RegistryKey string
}

// Browsers are the browsers `portcheck browser install` knows.
var Browsers = []Browser{
	{
		ID: "chrome", Name: "Google Chrome", RegistryKey: `Software\Google\Chrome`,
		Dir: map[string]string{"darwin": "Library/Application Support/Google/Chrome", "linux": ".config/google-chrome"},
	},
	{
		ID: "edge", Name: "Microsoft Edge", RegistryKey: `Software\Microsoft\Edge`,
		Dir: map[string]string{"darwin": "Library/Application Support/Microsoft Edge", "linux": ".config/microsoft-edge"},
	},
	{
		ID: "chromium", Name: "Chromium", RegistryKey: `Software\Chromium`,
		Dir: map[string]string{"darwin": "Library/Application Support/Chromium", "linux": ".config/chromium"},
	},
	{
		ID: "brave", Name: "Brave", RegistryKey: `Software\BraveSoftware\Brave-Browser`,
		Dir: map[string]string{"darwin": "Library/Application Support/BraveSoftware/Brave-Browser", "linux": ".config/BraveSoftware/Brave-Browser"},
	},
}

// Manifest is a native messaging host manifest.
type Manifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// NewManifest returns the manifest for the portcheck binary at path.
func NewManifest(path string, extensionIDs []string) Manifest {
	m := Manifest{
		Name:        HostName,
		Description: "portcheck: see what's using your ports (https://github.com/roystondwork-dev/portcheck)",
		Path:        path,
		Type:        "stdio",
	}
	for _, id := range extensionIDs {
		m.AllowedOrigins = append(m.AllowedOrigins, "chrome-extension://"+id+"/")
	}
	return m
}

// Registry is the subset of the Windows registry used to register hosts.
type Registry interface {
	// KeyExists reports whether HKEY_CURRENT_USER\key exists.
	KeyExists(key string) bool
	// SetDefault sets the default value of HKEY_CURRENT_USER\key, creating it.
	SetDefault(key, value string) error
	// DefaultValue returns the default value of HKEY_CURRENT_USER\key.
	DefaultValue(key string) (string, error)
	// DeleteKey deletes HKEY_CURRENT_USER\key; a missing key is not an error.
	DeleteKey(key string) error
}

// Env is the environment registration works in.
type Env struct {
	GOOS string
	// Home is the user's home directory (macOS, Linux).
	Home string
	// DataDir holds the manifest on Windows, e.g. %LOCALAPPDATA%\portcheck.
	DataDir  string
	Registry Registry
}

// Target is where a browser's host registration lives.
type Target struct {
	Browser Browser
	// Manifest is the manifest file's path.
	Manifest string
	// RegistryKey is the key pointing to it (Windows only).
	RegistryKey string
	// Detected reports whether the browser seems to be installed for this user.
	Detected bool
}

// Targets lists where each browser's registration lives in env.
func Targets(env Env) ([]Target, error) {
	var out []Target
	for _, b := range Browsers {
		t := Target{Browser: b}
		if env.GOOS == "windows" {
			if env.DataDir == "" || env.Registry == nil {
				return nil, errors.New("can't find the local app data directory or registry")
			}
			t.Manifest = filepath.Join(env.DataDir, "native-messaging", b.ID, HostName+".json")
			t.RegistryKey = b.RegistryKey + `\NativeMessagingHosts\` + HostName
			t.Detected = env.Registry.KeyExists(b.RegistryKey)
		} else {
			dir, ok := b.Dir[env.GOOS]
			if !ok {
				return nil, fmt.Errorf("browser registration isn't supported on %s", env.GOOS)
			}
			if env.Home == "" {
				return nil, errors.New("can't find the home directory")
			}
			base := filepath.Join(env.Home, filepath.FromSlash(dir))
			t.Manifest = filepath.Join(base, "NativeMessagingHosts", HostName+".json")
			t.Detected = isDir(base)
		}
		out = append(out, t)
	}
	return out, nil
}

// Select picks targets by browser ID; with no IDs, the detected browsers.
func Select(targets []Target, ids []string) ([]Target, error) {
	if len(ids) == 0 {
		var out []Target
		for _, t := range targets {
			if t.Detected {
				out = append(out, t)
			}
		}
		return out, nil
	}
	var out []Target
	for _, id := range ids {
		i := slices.IndexFunc(targets, func(t Target) bool { return t.Browser.ID == id })
		if i < 0 {
			return nil, fmt.Errorf("unknown browser %q (want chrome, edge, chromium or brave)", id)
		}
		out = append(out, targets[i])
	}
	return out, nil
}

// Install registers m with the browser at t.
func Install(env Env, t Target, m Manifest) error {
	data, _ := json.MarshalIndent(m, "", "  ") // only strings: can't fail
	// The browser runs as the same user, so owner-only permissions suffice.
	if err := os.MkdirAll(filepath.Dir(t.Manifest), 0o750); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(t.Manifest), err)
	}
	if err := os.WriteFile(t.Manifest, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", t.Manifest, err)
	}
	if t.RegistryKey != "" {
		if err := env.Registry.SetDefault(t.RegistryKey, t.Manifest); err != nil {
			return fmt.Errorf("register with %s: %w", t.Browser.Name, err)
		}
	}
	return nil
}

// Uninstall removes the registration at t; a missing one is not an error.
func Uninstall(env Env, t Target) error {
	if t.RegistryKey != "" {
		if err := env.Registry.DeleteKey(t.RegistryKey); err != nil {
			return fmt.Errorf("unregister from %s: %w", t.Browser.Name, err)
		}
	}
	if err := os.Remove(t.Manifest); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", t.Manifest, err)
	}
	return nil
}

// Installed reads the registration at t. It returns nil when there is none.
func Installed(env Env, t Target) (*Manifest, error) {
	path := t.Manifest
	if t.RegistryKey != "" {
		if !env.Registry.KeyExists(t.RegistryKey) {
			return nil, nil
		}
		v, err := env.Registry.DefaultValue(t.RegistryKey)
		if err != nil {
			return nil, fmt.Errorf("read the %s registration: %w", t.Browser.Name, err)
		}
		path = v
	}
	data, err := os.ReadFile(path) //nolint:gosec // our own registration
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return &m, nil
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// HostPath returns the binary path to register: the copy found on PATH when it
// is the running binary (e.g. /opt/homebrew/bin/portcheck, which survives
// upgrades), otherwise the running binary itself.
func HostPath(executable, onPath string) string {
	if onPath == "" {
		return executable
	}
	a, errA := os.Stat(executable)
	b, errB := os.Stat(onPath)
	if errA == nil && errB == nil && os.SameFile(a, b) {
		return onPath
	}
	return executable
}
