// Package portreg remembers a port for each project on this machine, so that
// `portcheck port` and `portcheck run --port auto` give a project the same port
// every time and two projects never get the same one.
//
// The registry is a small JSON file (ports.json) in portcheck's configuration
// directory, keyed by the project's root directory. Updates take a lock file,
// read, change and atomically replace the file, so concurrent invocations
// don't lose each other's assignments.
package portreg

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// FileName is the registry's file name in the configuration directory.
const FileName = "ports.json"

// formatVersion is the File.Version this package writes.
const formatVersion = 1

// File is the registry's contents.
type File struct {
	Version int `json:"version"`
	// Projects maps a project's root directory to its assignment.
	Projects map[string]Entry `json:"projects"`
}

// Entry is one project's assignment.
type Entry struct {
	Name     string    `json:"name"`
	Port     uint16    `json:"port"`
	LastUsed time.Time `json:"lastUsed"`
}

// Owner returns the project root, other than except, that port is assigned to.
func (f *File) Owner(port uint16, except string) (string, bool) {
	for root, e := range f.Projects {
		if e.Port == port && root != except {
			return root, true
		}
	}
	return "", false
}

// ErrLocked is returned when another portcheck holds the registry's lock for
// longer than the lock timeout.
var ErrLocked = errors.New("the port registry is locked by another portcheck")

// ErrNoPort is returned by Assign when every port in the range is assigned to
// another project or in use.
var ErrNoPort = errors.New("no unassigned free port")

// Registry is the registry file at Path.
type Registry struct {
	// Path is the registry file.
	Path string
	// Warn reports a corrupt registry file, which is set aside and replaced.
	Warn func(format string, a ...any)

	lockTimeout time.Duration // how long Update waits for the lock
	staleAfter  time.Duration // a lock older than this was left by a crash
	now         func() time.Time
}

// New returns the registry in the configuration directory dir.
func New(dir string, warn func(format string, a ...any)) *Registry {
	return &Registry{
		Path:        filepath.Join(dir, FileName),
		Warn:        warn,
		lockTimeout: 5 * time.Second,
		staleAfter:  30 * time.Second,
		now:         time.Now,
	}
}

// Read returns the registry, empty when the file doesn't exist yet. A corrupt
// file is reported through Warn and read as empty.
func (r *Registry) Read() (*File, error) {
	return r.load(false)
}

// Update locks the registry, reads it, calls fn and, if fn returns nil, writes
// the result back atomically. A corrupt file is reported, set aside as
// ports.json.corrupt and replaced.
func (r *Registry) Update(fn func(*File) error) error {
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(r.Path), err)
	}
	unlock, err := r.lock()
	if err != nil {
		return err
	}
	defer unlock()
	f, err := r.load(true)
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		return err
	}
	return r.write(f)
}

func (r *Registry) load(setAside bool) (*File, error) {
	empty := &File{Version: formatVersion, Projects: map[string]Entry{}}
	data, err := os.ReadFile(r.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the port registry: %w", err)
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil || f.Version != formatVersion {
		why := fmt.Sprintf("version %d", f.Version)
		if err != nil {
			why = err.Error()
		}
		if setAside {
			r.Warn("%s is unreadable (%s); starting a new one (the old file is kept as %s.corrupt)", r.Path, why, FileName)
			_ = os.Rename(r.Path, r.Path+".corrupt") // best effort: the next write replaces it anyway
		} else {
			r.Warn("%s is unreadable (%s); showing no assignments", r.Path, why)
		}
		return empty, nil
	}
	if f.Projects == nil {
		f.Projects = map[string]Entry{}
	}
	return &f, nil
}

// createTemp, rename and openLock are variables so tests can make them fail.
var (
	createTemp = os.CreateTemp
	rename     = os.Rename
	openLock   = func(path string) (*os.File, error) {
		return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // our own lock file in portcheck's config folder
	}
	// retryDenied is whether "access denied" while creating the lock means
	// the lock is being released (Windows), rather than a real permission error.
	retryDenied = runtime.GOOS == "windows"
)

// write replaces the registry file atomically: a temporary file in the same
// directory, renamed over the old one.
func (r *Registry) write(f *File) error {
	data, _ := json.MarshalIndent(f, "", "  ") // File always marshals
	tmp, err := createTemp(filepath.Dir(r.Path), ".ports-*.json")
	if err != nil {
		return fmt.Errorf("write the port registry: %w", err)
	}
	_, werr := tmp.Write(append(data, '\n'))
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write the port registry: %w", err)
	}
	if err := rename(tmp.Name(), r.Path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write the port registry: %w", err)
	}
	return nil
}

// lock takes the registry's lock file, which only one process can create. A
// lock left behind by a crashed portcheck is removed once it is stale.
func (r *Registry) lock() (unlock func(), err error) {
	path := r.Path + ".lock"
	deadline := r.now().Add(r.lockTimeout)
	for {
		lf, err := openLock(path)
		if err == nil {
			_ = lf.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		// On Windows, a lock file another process has just removed can't be
		// created again for a moment: that fails with "access denied", not
		// "exists". Wait for it like for a held lock.
		busy := errors.Is(err, fs.ErrExist) || (retryDenied && errors.Is(err, fs.ErrPermission))
		if !busy {
			return nil, fmt.Errorf("lock the port registry: %w", err)
		}
		if st, serr := os.Stat(path); serr == nil && r.now().Sub(st.ModTime()) > r.staleAfter {
			_ = os.Remove(path)
			continue
		}
		if r.now().After(deadline) {
			return nil, fmt.Errorf("%w (%s); remove it if no portcheck is running", ErrLocked, path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Preferred is where Assign starts looking for root's port: a hash of the
// root into spec, so a project tends to get the same port on every machine
// even before it is assigned. spec must not be empty.
func Preferred(root string, spec portspec.Spec) uint16 {
	p, _ := spec.At(preferredIndex(root, spec.Len()))
	return p
}

func preferredIndex(root string, n int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(root)) // hash writes never fail
	return int(h.Sum32() % uint32(n))
}

// Assign returns the first port in spec, starting at Preferred(root) and
// wrapping around, that f doesn't assign to another project and for which
// free reports true. spec must not be empty.
func Assign(f *File, root string, spec portspec.Spec, free func(uint16) (bool, error)) (uint16, error) {
	n := spec.Len()
	start := preferredIndex(root, n)
	for i := range n {
		p, _ := spec.At((start + i) % n)
		if _, taken := f.Owner(p, root); taken {
			continue
		}
		ok, err := free(p)
		if err != nil {
			return 0, err
		}
		if ok {
			return p, nil
		}
	}
	return 0, fmt.Errorf("%w in %s", ErrNoPort, spec)
}
