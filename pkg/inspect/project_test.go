package inspect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectName(t *testing.T) {
	t.Parallel()
	root := filepath.VolumeName(os.TempDir()) + string(filepath.Separator)
	home := filepath.Join(root, "home", "alice")
	shop := filepath.Join(home, "code", "shop")
	markers := map[string]bool{
		filepath.Join(shop, "package.json"):                  true,
		filepath.Join(home, "code", "api", ".git"):           true,
		filepath.Join(root, "srv", "app", "go.mod"):          true,
		filepath.Join(home, "deep", "a", "b", "c", "go.mod"): true,
	}
	exists := func(p string) bool { return markers[p] }
	deep := filepath.Join(home, "deep", "a", "b", "c")
	for i := range maxProjectDepth {
		deep = filepath.Join(deep, string(rune('d'+i)))
	}
	for _, tc := range []struct{ cwd, want string }{
		{"", ""},
		{root, ""},
		{home, ""},
		{shop, "shop"}, // the marker is in cwd
		{filepath.Join(shop, "web", "src"), "shop"},                          // found further up
		{filepath.Join(home, "code", "api", "cmd"), "api"},                   // .git counts
		{filepath.Join(root, "srv", "app"), "app"},                           // a marker counts outside home too
		{filepath.Join(home, "notes"), "notes"},                              // no marker: cwd's own name, inside home
		{filepath.Join(home, "notes") + string(filepath.Separator), "notes"}, // cleaned
		{filepath.Join(root, "var", "www"), ""},                              // no marker outside home: no project
		{filepath.Join(root, "Windows", "system32"), ""},                     // e.g. Windows services
		{deep, filepath.Base(deep)},                                          // gives up after maxProjectDepth
	} {
		if got := projectName(tc.cwd, home, exists); got != tc.want {
			t.Errorf("projectName(%q) = %q, want %q", tc.cwd, got, tc.want)
		}
	}
}

func TestWithin(t *testing.T) {
	t.Parallel()
	sep := string(filepath.Separator)
	home := sep + filepath.Join("home", "alice")
	upper := strings.ToUpper(home) + sep + "x"
	for _, tc := range []struct {
		dir, parent string
		fold, want  bool
	}{
		{home + sep + "x", home, false, true},
		{home, home, false, false},
		{home + "2" + sep + "x", home, false, false},
		{home + sep + "x", "", false, false},
		{sep + "tmp", home, false, false},
		{upper, home, false, false},
		{upper, home, true, true},
		{sep + "x", home, true, false},
	} {
		if got := within(tc.dir, tc.parent, tc.fold); got != tc.want {
			t.Errorf("within(%q, %q, %v) = %v, want %v", tc.dir, tc.parent, tc.fold, got, tc.want)
		}
	}
}

func TestProjectNameOnDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "cmd", "server")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := ProjectName(sub); got != filepath.Base(dir) {
		t.Errorf("ProjectName(%q) = %q, want %q", sub, got, filepath.Base(dir))
	}
}
