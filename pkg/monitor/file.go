package monitor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// FileName is the monitors file in portcheck's configuration folder.
const FileName = "monitors.yaml"

// Replaced in tests to reach the error paths.
var (
	marshal    = yaml.Marshal
	createTemp = os.CreateTemp
	writeFile  = os.WriteFile
	rename     = os.Rename
)

// File is the user's list of monitors.
type File struct {
	Version  int      `yaml:"version"`
	Monitors []Target `yaml:"monitors"`
}

// Load reads the monitors file at path. A missing file is an empty list.
func Load(path string) (*File, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the user's own monitors file in portcheck's config folder
	if errors.Is(err, fs.ErrNotExist) {
		return &File{Version: 1}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if f.Version == 0 {
		f.Version = 1
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("%s: version %d is not supported (expected 1); update portcheck", path, f.Version)
	}
	seen := map[string]bool{}
	for _, t := range f.Monitors {
		if err := t.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if seen[t.Name] {
			return nil, fmt.Errorf("%s: two monitors are named %q", path, t.Name)
		}
		seen[t.Name] = true
	}
	return &f, nil
}

// Save writes f to path, replacing the file in one step so a reader never
// sees half of it.
func Save(path string, f *File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	b, err := marshal(f)
	if err != nil {
		return fmt.Errorf("encode monitors: %w", err)
	}
	header := "# portcheck monitors: edit with `portcheck monitor add/remove`, or by hand.\n"
	tmp, err := createTemp(filepath.Dir(path), ".monitors-*.yaml")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	name := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(name) }()
	if err := writeFile(name, []byte(header+string(b)), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := rename(name, path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Add appends t, or reports why it can't be added.
func (f *File) Add(t Target) error {
	if err := t.Validate(); err != nil {
		return err
	}
	for _, m := range f.Monitors {
		if m.Name == t.Name {
			return fmt.Errorf("a monitor named %q already exists; remove it first or pick another name with --name", t.Name)
		}
	}
	f.Monitors = append(f.Monitors, t)
	return nil
}

// Remove deletes the monitor with this name, URL or address (ignoring a
// trailing slash), and returns it.
func (f *File) Remove(key string) (Target, bool) {
	norm := func(s string) string { return strings.TrimSuffix(s, "/") }
	for i, m := range f.Monitors {
		if m.Name == key || norm(m.Endpoint()) == norm(key) {
			f.Monitors = append(f.Monitors[:i], f.Monitors[i+1:]...)
			return m, true
		}
	}
	return Target{}, false
}
