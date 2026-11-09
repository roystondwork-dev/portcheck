package portreg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

type warnings struct {
	mu   sync.Mutex
	msgs []string
}

func (w *warnings) warn(format string, a ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.msgs = append(w.msgs, fmt.Sprintf(format, a...))
}

func newReg(t *testing.T) (*Registry, *warnings) {
	t.Helper()
	w := &warnings{}
	return New(filepath.Join(t.TempDir(), "portcheck"), w.warn), w
}

func TestReadMissing(t *testing.T) {
	t.Parallel()
	r, _ := newReg(t)
	f, err := r.Read()
	if err != nil || f.Version != 1 || len(f.Projects) != 0 {
		t.Fatalf("Read() = %+v, %v", f, err)
	}
}

func TestUpdateAndRead(t *testing.T) {
	t.Parallel()
	r, _ := newReg(t)
	when := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	if err := r.Update(func(f *File) error {
		f.Projects["/src/shop"] = Entry{Name: "shop", Port: 3456, LastUsed: when}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Projects["/src/shop"]; got.Port != 3456 || got.Name != "shop" || !got.LastUsed.Equal(when) {
		t.Errorf("entry = %+v", got)
	}
	if root, ok := f.Owner(3456, "/src/api"); !ok || root != "/src/shop" {
		t.Errorf("Owner = %q, %v", root, ok)
	}
	if _, ok := f.Owner(3456, "/src/shop"); ok {
		t.Error("Owner counted the excepted project")
	}
	// No lock file or temporary files are left behind.
	entries, _ := os.ReadDir(filepath.Dir(r.Path))
	if len(entries) != 1 || entries[0].Name() != FileName {
		t.Errorf("directory holds %v", entries)
	}
}

func TestUpdateErrorWritesNothing(t *testing.T) {
	t.Parallel()
	r, _ := newReg(t)
	boom := errors.New("boom")
	if err := r.Update(func(f *File) error {
		f.Projects["/x"] = Entry{Port: 1}
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("Update = %v", err)
	}
	if _, err := os.Stat(r.Path); !os.IsNotExist(err) {
		t.Errorf("registry written despite the error: %v", err)
	}
}

func TestCorrupt(t *testing.T) {
	t.Parallel()
	for name, content := range map[string]string{"not json": "{nope", "future version": `{"version": 2, "projects": {}}`} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r, w := newReg(t)
			if err := os.MkdirAll(filepath.Dir(r.Path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(r.Path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			f, err := r.Read()
			if err != nil || len(f.Projects) != 0 || len(w.msgs) != 1 || !strings.Contains(w.msgs[0], "showing no assignments") {
				t.Fatalf("Read() = %+v, %v; warnings %q", f, err, w.msgs)
			}
			if err := r.Update(func(f *File) error {
				f.Projects["/a"] = Entry{Name: "a", Port: 3001}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if len(w.msgs) != 2 || !strings.Contains(w.msgs[1], "starting a new one") {
				t.Errorf("warnings = %q", w.msgs)
			}
			if old, err := os.ReadFile(r.Path + ".corrupt"); err != nil || string(old) != content {
				t.Errorf("corrupt file kept as %q, %v", old, err)
			}
			if f, _ := r.Read(); f.Projects["/a"].Port != 3001 {
				t.Errorf("after Update: %+v", f)
			}
		})
	}
}

func TestNullProjects(t *testing.T) {
	t.Parallel()
	r, _ := newReg(t)
	_ = os.MkdirAll(filepath.Dir(r.Path), 0o700)
	_ = os.WriteFile(r.Path, []byte(`{"version": 1}`), 0o600)
	f, err := r.Read()
	if err != nil || f.Projects == nil {
		t.Fatalf("Read() = %+v, %v", f, err)
	}
}

func TestReadError(t *testing.T) {
	t.Parallel()
	r, _ := newReg(t)
	if err := os.MkdirAll(r.Path, 0o700); err != nil { // a directory where the file should be
		t.Fatal(err)
	}
	if _, err := r.Read(); err == nil {
		t.Error("Read succeeded on a directory")
	}
	if err := r.Update(func(*File) error { return nil }); err == nil {
		t.Error("Update succeeded on a directory")
	}
}

func TestUpdateCannotCreateDir(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	blocker := filepath.Join(parent, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r := New(filepath.Join(blocker, "portcheck"), nil)
	if err := r.Update(func(*File) error { return nil }); err == nil || !strings.Contains(err.Error(), "create") {
		t.Errorf("Update = %v", err)
	}
}

func TestConcurrentUpdates(t *testing.T) {
	t.Parallel()
	r, _ := newReg(t)
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Go(func() {
			errs <- r.Update(func(f *File) error {
				f.Projects[fmt.Sprintf("/p%d", i)] = Entry{Name: strconv.Itoa(i), Port: uint16(3000 + i)}
				return nil
			})
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if f, _ := r.Read(); len(f.Projects) != n {
		t.Errorf("%d projects after %d concurrent updates", len(f.Projects), n)
	}
}

func TestLock(t *testing.T) {
	t.Parallel()
	t.Run("stale lock is removed", func(t *testing.T) {
		t.Parallel()
		r, _ := newReg(t)
		_ = os.MkdirAll(filepath.Dir(r.Path), 0o700)
		lock := r.Path + ".lock"
		if err := os.WriteFile(lock, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-time.Hour)
		if err := os.Chtimes(lock, old, old); err != nil {
			t.Fatal(err)
		}
		if err := r.Update(func(*File) error { return nil }); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("held lock times out", func(t *testing.T) {
		t.Parallel()
		r, _ := newReg(t)
		r.lockTimeout = 50 * time.Millisecond
		_ = os.MkdirAll(filepath.Dir(r.Path), 0o700)
		if err := os.WriteFile(r.Path+".lock", nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := r.Update(func(*File) error { return nil }); !errors.Is(err, ErrLocked) {
			t.Errorf("Update = %v, want ErrLocked", err)
		}
	})
	t.Run("lock can't be created", func(t *testing.T) {
		t.Parallel()
		r, _ := newReg(t)
		if err := os.MkdirAll(r.Path+".lock"+string(filepath.Separator)+"x", 0o700); err != nil {
			t.Fatal(err)
		}
		// The lock path is a non-empty directory, so creating it fails and
		// removing it as stale fails too: the lock is never taken.
		r.lockTimeout, r.staleAfter = 50*time.Millisecond, time.Hour
		// ErrLocked on Unix; Windows refuses to open a directory with its own error.
		if err := r.Update(func(*File) error { return nil }); err == nil {
			t.Error("Update took a lock that is a directory")
		}
	})
}

func TestLockOpenError(t *testing.T) {
	t.Parallel()
	r := &Registry{Path: filepath.Join(t.TempDir(), "missing", FileName), lockTimeout: time.Second, staleAfter: time.Second, now: time.Now}
	if _, err := r.lock(); err == nil || errors.Is(err, ErrLocked) {
		t.Errorf("lock() = %v, want an open error", err)
	}
}

// Not parallel: it swaps the package's createTemp and rename.
func TestWriteErrors(t *testing.T) {
	boom := errors.New("boom")
	t.Run("create", func(t *testing.T) {
		createTemp = func(string, string) (*os.File, error) { return nil, boom }
		defer func() { createTemp = os.CreateTemp }()
		r, _ := newReg(t)
		if err := r.Update(func(*File) error { return nil }); !errors.Is(err, boom) {
			t.Errorf("Update = %v", err)
		}
	})
	t.Run("write", func(t *testing.T) {
		createTemp = func(dir, pattern string) (*os.File, error) {
			f, err := os.CreateTemp(dir, pattern)
			if err == nil {
				_ = f.Close() // writing to a closed file fails
			}
			return f, err
		}
		defer func() { createTemp = os.CreateTemp }()
		r, _ := newReg(t)
		if err := r.Update(func(*File) error { return nil }); err == nil {
			t.Error("Update succeeded")
		}
		if entries, _ := os.ReadDir(filepath.Dir(r.Path)); len(entries) != 0 {
			t.Errorf("left behind %v", entries)
		}
	})
	t.Run("rename", func(t *testing.T) {
		rename = func(string, string) error { return boom }
		defer func() { rename = os.Rename }()
		r, _ := newReg(t)
		if err := r.Update(func(*File) error { return nil }); !errors.Is(err, boom) {
			t.Errorf("Update = %v", err)
		}
		if entries, _ := os.ReadDir(filepath.Dir(r.Path)); len(entries) != 0 {
			t.Errorf("left behind %v", entries)
		}
	})
}

func TestAssign(t *testing.T) {
	t.Parallel()
	spec := portspec.MustParse("3000-3009")
	free := func(busy ...uint16) func(uint16) (bool, error) {
		return func(p uint16) (bool, error) {
			for _, b := range busy {
				if p == b {
					return false, nil
				}
			}
			return true, nil
		}
	}
	f := &File{Projects: map[string]Entry{}}
	pref := Preferred("/src/shop", spec)
	if !spec.Contains(pref) || Preferred("/src/shop", spec) != pref {
		t.Fatalf("Preferred = %d, not stable or not in range", pref)
	}
	if p, err := Assign(f, "/src/shop", spec, free()); err != nil || p != pref {
		t.Errorf("Assign = %d, %v; want the preferred %d", p, err, pref)
	}
	// Assigned to another project, or in use: the next one, wrapping around.
	f.Projects["/src/api"] = Entry{Port: pref}
	next := pref + 1
	if next > 3009 {
		next = 3000
	}
	after := next + 1
	if after > 3009 {
		after = 3000
	}
	if p, _ := Assign(f, "/src/shop", spec, free(next)); p != after {
		t.Errorf("Assign = %d, want %d (skipping %d assigned and %d busy)", p, after, pref, next)
	}
	// Its own assignment doesn't block it.
	f.Projects["/src/shop"] = Entry{Port: pref}
	delete(f.Projects, "/src/api")
	if p, _ := Assign(f, "/src/shop", spec, free()); p != pref {
		t.Errorf("Assign = %d, want its own %d", p, pref)
	}
	// Nothing left.
	if _, err := Assign(f, "/src/x", portspec.Single(3000), free(3000)); !errors.Is(err, ErrNoPort) {
		t.Errorf("Assign = %v, want ErrNoPort", err)
	}
	boom := errors.New("boom")
	if _, err := Assign(f, "/src/x", spec, func(uint16) (bool, error) { return false, boom }); !errors.Is(err, boom) {
		t.Errorf("Assign = %v, want boom", err)
	}
}

func TestPreferredSpreads(t *testing.T) {
	t.Parallel()
	spec := portspec.MustParse("3000-3999")
	seen := map[uint16]bool{}
	for i := range 50 {
		seen[Preferred(fmt.Sprintf("/home/alice/code/project-%d", i), spec)] = true
	}
	if len(seen) < 40 {
		t.Errorf("50 projects share only %d preferred ports", len(seen))
	}
}
