// Package project loads a project's port configuration, .portcheck.yaml.
//
// The file lists the ports a project needs and what state each should be in:
//
//	version: 1
//	ports:
//	  - name: web
//	    port: 3000            # expect: free (default), so the dev server can bind it
//	  - name: postgres
//	    port: 5432
//	    expect: open          # must accept connections (e.g. started by docker compose)
//
// It is found by walking up from the working directory, like .editorconfig.
package project

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// FileNames are the accepted config file names, in order of preference.
var FileNames = []string{".portcheck.yaml", ".portcheck.yml"}

// SchemaURL is the published JSON Schema for the config file.
const SchemaURL = "https://roystondwork-dev.github.io/portcheck/schema/portcheck.schema.json"

// Errors returned (wrapped) by Find and Load.
var (
	ErrNotFound = errors.New("no .portcheck.yaml found")
	ErrInvalid  = errors.New("invalid .portcheck.yaml")
)

// Expectations.
const (
	ExpectFree = "free"
	ExpectOpen = "open"
)

// Config is a parsed .portcheck.yaml.
type Config struct {
	Version int    `yaml:"version"`
	Ports   []Port `yaml:"ports"`
	// Path is the file the config was loaded from.
	Path string `yaml:"-"`
}

// Port is one entry of the config.
type Port struct {
	// Name identifies the entry, e.g. "web"; it can be used with `run --port`.
	Name string `yaml:"name"`
	// Port is the port number.
	Port int `yaml:"port"`
	// Proto is "tcp" (default) or "udp".
	Proto string `yaml:"proto"`
	// Expect is "free" (default): nothing may listen, so the project can bind
	// it; or "open": a service must accept connections.
	Expect string `yaml:"expect"`
	// Host is where an "open" port is checked (default "localhost").
	Host string `yaml:"host"`
	// Description is shown in doctor output.
	Description string `yaml:"description"`
}

var (
	nameRE     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	typeNameRE = regexp.MustCompile(` in type [\w.\[\]]+`)
)

// Find returns the path of the nearest config file in dir or its parents.
// dir should be absolute (e.g. from os.Getwd) so that parents can be searched.
func Find(dir string) (string, error) {
	dir = filepath.Clean(dir)
	start := dir
	for {
		for _, name := range FileNames {
			p := filepath.Join(dir, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w in %s or any parent directory", ErrNotFound, start)
		}
		dir = parent
	}
}

// Load reads and validates the config at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // reading the user's own config file is the purpose
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	cfg.Path = path
	return cfg, nil
}

// Parse decodes and validates config data. Unknown keys are errors, so typos
// such as "exepct" are caught. Defaults are applied to the result.
func Parse(data []byte) (*Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: the file is empty", ErrInvalid)
		}
		// Drop Go type names ("in type project.Port") from YAML errors.
		msg := typeNameRE.ReplaceAllString(err.Error(), "")
		return nil, fmt.Errorf("%w: %s", ErrInvalid, msg)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Version != 1 {
		return invalid("version must be 1 (got %d)", c.Version)
	}
	if len(c.Ports) == 0 {
		return invalid("ports must list at least one port")
	}
	seen := map[string]bool{}
	for i := range c.Ports {
		p := &c.Ports[i]
		where := fmt.Sprintf("ports[%d]", i)
		if p.Name != "" {
			where += " (" + p.Name + ")"
		}
		switch {
		case p.Name == "":
			return invalid("%s: name is required", where)
		case !nameRE.MatchString(p.Name) || isDigits(p.Name):
			return invalid("%s: name must contain letters, digits, '.', '_' or '-', start with a letter or digit, and not be only digits", where)
		case seen[p.Name]:
			return invalid("%s: duplicate name", where)
		case p.Port < 1 || p.Port > 65535:
			return invalid("%s: port must be between 1 and 65535", where)
		}
		seen[p.Name] = true

		p.Proto = strings.ToLower(defaultTo(p.Proto, "tcp"))
		p.Expect = strings.ToLower(defaultTo(p.Expect, ExpectFree))
		switch {
		case p.Proto != "tcp" && p.Proto != "udp":
			return invalid("%s: proto must be tcp or udp", where)
		case p.Expect != ExpectFree && p.Expect != ExpectOpen:
			return invalid("%s: expect must be free or open", where)
		case p.Expect == ExpectOpen && p.Proto == "udp":
			return invalid("%s: expect: open is only supported for tcp", where)
		case p.Host != "" && p.Expect != ExpectOpen:
			return invalid("%s: host only applies to expect: open (free ports are checked locally)", where)
		}
		if p.Expect == ExpectOpen {
			p.Host = defaultTo(p.Host, "localhost")
		}
	}
	return nil
}

// Lookup returns the entry called name.
func (c *Config) Lookup(name string) (Port, bool) {
	for _, p := range c.Ports {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Port{}, false
}

// Names lists the entry names in file order.
func (c *Config) Names() []string {
	out := make([]string, len(c.Ports))
	for i, p := range c.Ports {
		out[i] = p.Name
	}
	return out
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func defaultTo(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func isDigits(s string) bool { return strings.Trim(s, "0123456789") == "" }
