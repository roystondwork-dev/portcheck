package inspect

import (
	"os"
	"path/filepath"
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
		{filepath.Join(shop, "web", "src"), "shop"},                             // found further up
		{filepath.Join(home, "code", "api", "cmd"), "api"},                      // .git counts
		{filepath.Join(root, "srv", "app"), "app"},                              // outside home
		{filepath.Join(home, "notes"), "notes"},                                 // no marker: cwd's own name
		{filepath.Join(root, "var", "www") + string(filepath.Separator), "www"}, // cleaned
		{deep, filepath.Base(deep)},                                             // gives up after maxProjectDepth
	} {
		if got := projectName(tc.cwd, home, exists); got != tc.want {
			t.Errorf("projectName(%q) = %q, want %q", tc.cwd, got, tc.want)
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
