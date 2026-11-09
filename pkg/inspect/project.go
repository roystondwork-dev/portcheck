package inspect

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// projectMarkers are the files and directories that mark a project's root.
var projectMarkers = []string{
	".git", "package.json", "go.mod", "pyproject.toml", "Cargo.toml", "Gemfile",
	"pom.xml", "build.gradle", "build.gradle.kts", "composer.json", "deno.json", "mix.exs",
}

// maxProjectDepth bounds how far up from the working directory ProjectRoot looks.
const maxProjectDepth = 8

// ProjectName names the project a process runs in, from its working directory:
// the name of the nearest directory at or above cwd that holds a project marker
// (.git, package.json, go.mod, …), or of cwd itself when there is none. It
// returns "" when cwd is empty, the filesystem root or the user's home
// directory, which name no project (daemons usually run in /), and, without a
// marker, when cwd is outside the user's home directory.
func ProjectName(cwd string) string {
	if root := ProjectRoot(cwd); root != "" {
		return filepath.Base(root)
	}
	return ""
}

// ProjectRoot is the directory ProjectName names: the nearest directory at or
// above cwd that holds a project marker, or cwd itself when there is none and
// cwd is inside the home directory. It returns "" otherwise, and for an empty
// cwd, the filesystem root and the home directory.
func ProjectRoot(cwd string) string {
	home, _ := os.UserHomeDir()
	return projectRoot(cwd, home, func(path string) bool {
		_, err := os.Lstat(path)
		return err == nil
	})
}

func projectRoot(cwd, home string, exists func(string) bool) string {
	if cwd == "" {
		return ""
	}
	cwd = filepath.Clean(cwd)
	isTop := func(dir string) bool { return dir == home || filepath.Dir(dir) == dir }
	if isTop(cwd) {
		return ""
	}
	dir := cwd
	for range maxProjectDepth {
		for _, m := range projectMarkers {
			if exists(filepath.Join(dir, m)) {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if isTop(parent) {
			break
		}
		dir = parent
	}
	// Without a marker, only a folder of the user's own counts: system and
	// service processes run in places like C:\Windows\system32 or /var/lib.
	if !within(cwd, home, foldCase) {
		return ""
	}
	return cwd
}

// foldCase is whether paths compare case-insensitively: Windows and macOS file
// systems usually ignore case.
var foldCase = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

// within reports whether dir is strictly inside parent.
func within(dir, parent string, fold bool) bool {
	if parent == "" {
		return false
	}
	prefix := filepath.Clean(parent) + string(filepath.Separator)
	if fold {
		return len(dir) > len(prefix) && strings.EqualFold(dir[:len(prefix)], prefix)
	}
	return strings.HasPrefix(dir, prefix)
}
