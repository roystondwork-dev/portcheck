// Package cmdports guesses which local ports a shell command line listens on.
//
// The shell hook (`portcheck init`) can't see a failed command's error output,
// only its command line, so it asks this package which ports to check: ports
// given on the command line (--port 3000, -p 8080:80, PORT=4000, 0.0.0.0:8000,
// …), or else the default port of a known dev server (vite → 5173, rails
// server → 3000, …). Package scripts (`npm run dev`) are expanded from
// package.json and read with the same rules.
//
// It is a heuristic by design: it reads ports, it never runs anything.
package cmdports

import (
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Source says how a port was found.
type Source string

const (
	// Explicit ports appear on the command line or in an environment variable.
	Explicit Source = "explicit"
	// Default ports are a known tool's default, used when none is given.
	Default Source = "default"
)

// Candidate is a port a command probably listens on.
type Candidate struct {
	Port   uint16 `json:"port"`
	Source Source `json:"source"`
}

// Scripts looks up a package script by name, e.g. "dev" → "vite --port 3000".
// It returns "" when there is no such script.
type Scripts func(name string) string

// maxScriptDepth bounds script expansion (a script running another script, …).
const maxScriptDepth = 3

// Ports returns the ports cmdline probably listens on, in the order found,
// without duplicates. scripts may be nil.
func Ports(cmdline string, scripts Scripts) []Candidate {
	var out []Candidate
	add := func(cs []Candidate) {
		for _, c := range cs {
			if !slices.ContainsFunc(out, func(o Candidate) bool { return o.Port == c.Port }) {
				out = append(out, c)
			}
		}
	}
	for _, seg := range segments(split(cmdline)) {
		add(segment(seg, scripts, 0))
	}
	return out
}

// segment reads one simple command: VAR=value… program args….
func segment(words []string, scripts Scripts, depth int) []Candidate {
	var env []Candidate
	for len(words) > 0 && isAssignment(words[0]) {
		name, value, _ := strings.Cut(words[0], "=")
		if p, ok := portValue(value); ok && strings.HasSuffix(strings.ToUpper(name), "PORT") {
			env = append(env, Candidate{p, Explicit})
		}
		words = words[1:]
	}
	words = unwrap(words)
	if len(words) == 0 {
		return env
	}
	tool, args := program(words)
	if c := expandScript(tool, args, scripts, depth); c != nil {
		return append(env, c...)
	}
	if clients[tool] {
		return env // a port flag here is where it connects to, not where it listens
	}
	env = append(env, explicit(tool, args)...)
	if len(env) > 0 {
		return env
	}
	if p, ok := defaultPort(tool, args); ok {
		return []Candidate{{p, Default}}
	}
	return nil
}

// expandScript reads `npm run dev` and friends as the package script they run.
// It returns nil when words don't run a package script.
func expandScript(tool string, args []string, scripts Scripts, depth int) []Candidate {
	name, ok := scriptName(tool, args)
	if !ok {
		return nil
	}
	if scripts != nil && depth < maxScriptDepth {
		if text := scripts(name); text != "" {
			var out []Candidate
			for _, seg := range segments(split(text)) {
				out = append(out, segment(seg, scripts, depth+1)...)
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	if name == "start" {
		// `npm start` without a recognisable script: Create React App, Express
		// and most Node servers default to 3000.
		return []Candidate{{3000, Default}}
	}
	return []Candidate{}
}

// scriptName returns the package script a package manager command runs.
func scriptName(tool string, args []string) (string, bool) {
	switch tool {
	case "npm", "pnpm", "yarn", "bun":
	default:
		return "", false
	}
	pos := positionals(args)
	if len(pos) == 0 {
		return "", false
	}
	switch first := pos[0]; {
	case first == "run" || first == "run-script":
		if len(pos) > 1 {
			return pos[1], true
		}
		return "", false
	case first == "start" || first == "dev" || first == "serve":
		return first, true
	case tool == "yarn" || tool == "pnpm":
		// `yarn dev`, `pnpm dev`: any word that isn't a built-in runs a script.
		if !builtins[first] {
			return first, true
		}
	}
	return "", false
}

// builtins are package manager commands that don't run a script.
var builtins = map[string]bool{
	"install": true, "i": true, "add": true, "remove": true, "rm": true, "test": true, "t": true,
	"exec": true, "dlx": true, "create": true, "init": true, "build": true, "publish": true,
	"update": true, "upgrade": true, "up": true, "outdated": true, "why": true, "list": true,
	"ls": true, "info": true, "link": true, "unlink": true, "audit": true, "cache": true,
	"config": true, "global": true, "pack": true, "version": true, "help": true, "x": true,
}

// clients connect to a port given on their command line rather than listen on it.
var clients = map[string]bool{
	"ssh": true, "scp": true, "sftp": true, "mosh": true, "psql": true, "pg_isready": true,
	"pg_dump": true, "mysql": true, "mysqladmin": true, "mysqldump": true, "redis-cli": true,
	"mongosh": true, "mongo": true, "nc": true, "ncat": true, "netcat": true, "telnet": true,
	"curl": true, "wget": true, "http": true, "https": true, "xh": true, "ping": true,
	"grpcurl": true, "openssl": true, "nmap": true, "portcheck": true,
}

// wrappers run the command that follows them: `sudo vite`, `npx next dev`, ….
var wrappers = map[string]int{
	// The value is how many words the wrapper itself takes.
	"sudo": 1, "env": 1, "time": 1, "nohup": 1, "exec": 1, "command": 1, "npx": 1, "bunx": 1,
	"pnpx": 1, "dotenv": 1, "doppler": 1,
}

// pairs are two-word wrappers: `pnpm exec vite`, `uv run uvicorn`, ….
var pairs = map[[2]string]bool{
	{"pnpm", "exec"}: true, {"pnpm", "dlx"}: true, {"yarn", "dlx"}: true, {"yarn", "exec"}: true,
	{"npm", "exec"}: true, {"uv", "run"}: true, {"poetry", "run"}: true, {"pipenv", "run"}: true,
	{"bundle", "exec"}: true, {"pdm", "run"}: true, {"rye", "run"}: true, {"hatch", "run"}: true,
}

// unwrap drops wrapper commands and their options from the front of words.
func unwrap(words []string) []string {
	for len(words) > 0 {
		base := filepath.Base(words[0])
		switch {
		case wrappers[base] > 0:
			words = words[wrappers[base]:]
			// Options and VAR=value of the wrapper itself (sudo -E, env -i A=b, npx -y, dotenv --).
			for len(words) > 0 && (strings.HasPrefix(words[0], "-") || isAssignment(words[0])) {
				words = words[1:]
			}
		case len(words) > 1 && pairs[[2]string{base, words[1]}]:
			words = words[2:]
		default:
			return words
		}
	}
	return words
}

// program names the tool a command runs and returns its arguments: "vite" for
// ./node_modules/.bin/vite, "http.server" for python3 -m http.server,
// "manage.py" for python manage.py, "artisan" for php artisan.
func program(words []string) (string, []string) {
	tool, args := filepath.Base(words[0]), words[1:]
	switch {
	case isPython(tool):
		for i := 0; i < len(args); i++ {
			switch a := args[i]; {
			case a == "-m" && i+1 < len(args):
				return args[i+1], args[i+2:]
			case strings.HasPrefix(a, "-"):
			default:
				return filepath.Base(a), args[i+1:]
			}
		}
	case tool == "node" || tool == "deno" || (tool == "php" && len(args) > 0 && !strings.HasPrefix(args[0], "-")):
		for i, a := range args {
			if !strings.HasPrefix(a, "-") {
				if tool == "deno" && (a == "run" || a == "task") {
					continue
				}
				name := strings.TrimSuffix(filepath.Base(a), ".js")
				name = strings.TrimPrefix(name, "npm:") // deno run npm:vite
				return name, args[i+1:]
			}
		}
	case tool == "docker" || tool == "podman" || tool == "nerdctl" || tool == "kubectl":
		if pos := positionals(args); len(pos) > 0 {
			return tool + " " + pos[0], args
		}
	}
	return tool, args
}

var pythonName = regexp.MustCompile(`^(python[0-9.]*|py|pypy[0-9.]*)(\.exe)?$`)

func isPython(tool string) bool { return pythonName.MatchString(tool) }

// listenFlags take where to listen as their value.
var listenFlags = map[string]bool{
	"--port": true, "-p": true, "--listen": true, "--bind": true, "-b": true, "--http-port": true,
	"--server-port": true, "--server.port": true, "-S": true, "--publish": true,
}

// explicit finds ports given on the command line.
func explicit(tool string, args []string) []Candidate {
	docker := tool == "docker run" || tool == "podman run" || tool == "nerdctl run" ||
		tool == "docker create" || tool == "podman create"
	var out []Candidate
	add := func(v string) {
		if p, ok := portValue(v); ok {
			out = append(out, Candidate{p, Explicit})
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, value, hasValue := strings.Cut(a, "=")
		switch {
		case docker:
			if name == "-p" || name == "--publish" {
				if !hasValue && i+1 < len(args) {
					value, i = args[i+1], i+1
				}
				add(dockerHostPort(value))
			} else if strings.HasPrefix(a, "-p") && len(a) > 2 {
				add(dockerHostPort(a[2:]))
			}
		case listenFlags[name] && hasValue:
			add(value)
		case listenFlags[name] && i+1 < len(args):
			add(args[i+1])
			i++
		case tool == "serve" && (a == "-l" || a == "--listen") && i+1 < len(args):
			add(args[i+1])
			i++
		case strings.HasPrefix(a, "-p") && len(a) > 2 && isDigits(a[2:]):
			add(a[2:])
		case !strings.HasPrefix(a, "-"):
			if v, ok := listenAddress(a); ok {
				add(v)
			} else if positionalPort[tool] && isDigits(a) {
				add(a)
			} else if tool == "kubectl port-forward" {
				if local, _, found := strings.Cut(a, ":"); found && isDigits(local) {
					add(local)
				} else if isDigits(a) {
					add(a)
				}
			}
		}
	}
	return out
}

// positionalPort tools take a bare port number as an argument.
var positionalPort = map[string]bool{
	"http.server": true, "SimpleHTTPServer": true, "manage.py": true,
}

var addressRE = regexp.MustCompile(`^(localhost|0\.0\.0\.0|127\.0\.0\.1|\*|\[::\]|\[::1\]|::)?:([0-9]{1,5})$`)

// listenAddress reads a local listen address such as 0.0.0.0:8000, :8080 or
// localhost:3000, and returns its port.
func listenAddress(s string) (string, bool) {
	m := addressRE.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return m[2], true
}

// dockerHostPort returns the host side of a -p mapping: 8080 for 8080:80 and
// 127.0.0.1:8080:80, and nothing for a container port alone (80).
func dockerHostPort(v string) string {
	parts := strings.Split(strings.TrimSuffix(strings.TrimSuffix(v, "/tcp"), "/udp"), ":")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

// portValue reads a port from a flag value: 3000, :3000, 0.0.0.0:3000.
func portValue(v string) (uint16, bool) {
	if i := strings.LastIndexByte(v, ':'); i >= 0 {
		v = v[i+1:]
	}
	if !isDigits(v) {
		return 0, false
	}
	n, err := strconv.ParseUint(v, 10, 16)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint16(n), true
}

// defaultPort is the port a known dev server listens on when none is given.
func defaultPort(tool string, args []string) (uint16, bool) {
	pos := positionals(args)
	for _, d := range defaults {
		if d.tool != tool {
			continue
		}
		// Options with values (flask --app hello run) put words before the
		// subcommand, so any positional argument may be it.
		if len(d.subs) == 0 || (len(pos) == 0 && slices.Contains(d.subs, "")) ||
			slices.ContainsFunc(pos, func(p string) bool { return p != "" && slices.Contains(d.subs, p) }) {
			return d.port, true
		}
	}
	return 0, false
}

var defaults = []struct {
	tool string
	subs []string // the first argument, if it matters; "" means none
	port uint16
}{
	{"vite", []string{"", "dev", "serve"}, 5173},
	{"vite", []string{"preview"}, 4173},
	{"next", []string{"dev", "start"}, 3000},
	{"nuxt", []string{"dev"}, 3000},
	{"nuxi", []string{"dev"}, 3000},
	{"react-scripts", []string{"start"}, 3000},
	{"rails", []string{"s", "server"}, 3000},
	{"flask", []string{"run"}, 5000},
	{"manage.py", []string{"runserver"}, 8000},
	{"uvicorn", nil, 8000},
	{"gunicorn", nil, 8000},
	{"fastapi", []string{"dev", "run"}, 8000},
	{"hugo", []string{"server", "serve"}, 1313},
	{"jekyll", []string{"serve", "s", "server"}, 4000},
	{"astro", []string{"dev", "preview"}, 4321},
	{"ng", []string{"serve", "s"}, 4200},
	{"gatsby", []string{"develop"}, 8000},
	{"artisan", []string{"serve"}, 8000},
	{"mkdocs", []string{"serve"}, 8000},
	{"http.server", nil, 8000},
	{"SimpleHTTPServer", nil, 8000},
	{"serve", nil, 3000},
}

// positionals returns the arguments that aren't options (skipping each
// option's value only when it's written as --opt=value).
func positionals(args []string) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
		}
	}
	return out
}

func isAssignment(w string) bool {
	name, _, ok := strings.Cut(w, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
