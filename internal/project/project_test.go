package project

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const example = `# yaml-language-server: $schema=https://roystondwork-dev.github.io/portcheck/schema/portcheck.schema.json
version: 1
ports:
  - name: web
    port: 3000
    description: Next.js dev server
  - name: postgres
    port: 5432
    expect: OPEN
  - name: redis
    port: 6379
    expect: open
    host: 127.0.0.1
  - name: dns.local
    port: 5353
    proto: UDP
`

func TestParse(t *testing.T) {
	t.Parallel()
	cfg, err := Parse([]byte(example))
	if err != nil {
		t.Fatal(err)
	}
	want := []Port{
		{Name: "web", Port: 3000, Proto: "tcp", Expect: "free", Description: "Next.js dev server"},
		{Name: "postgres", Port: 5432, Proto: "tcp", Expect: "open", Host: "localhost"},
		{Name: "redis", Port: 6379, Proto: "tcp", Expect: "open", Host: "127.0.0.1"},
		{Name: "dns.local", Port: 5353, Proto: "udp", Expect: "free"},
	}
	if d := cmp.Diff(want, cfg.Ports); d != "" {
		t.Errorf("(-want +got):\n%s", d)
	}
	if !slices.Equal(cfg.Names(), []string{"web", "postgres", "redis", "dns.local"}) {
		t.Errorf("Names = %v", cfg.Names())
	}
	if p, ok := cfg.Lookup("POSTGRES"); !ok || p.Port != 5432 {
		t.Errorf("Lookup is case-insensitive: %v %v", p, ok)
	}
	if _, ok := cfg.Lookup("nope"); ok {
		t.Error("Lookup found a missing name")
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	entry := func(s string) string { return "version: 1\nports:\n  - " + s + "\n" }
	tests := []struct {
		name, yaml, want string
	}{
		{"empty", "", "the file is empty"},
		{"not yaml", "version: [", "invalid .portcheck.yaml"},
		{"typo key", "version: 1\nports:\n  - name: web\n    port: 3000\n    exepct: open\n", "line 5: field exepct not found"},
		{"bad version", "version: 2\nports:\n  - name: a\n    port: 1\n", "version must be 1 (got 2)"},
		{"no ports", "version: 1\nports: []\n", "at least one port"},
		{"no name", entry("port: 1"), "ports[0]: name is required"},
		{"bad name", entry("name: -web\n    port: 1"), "ports[0] (-web): name must"},
		{"digit name", entry("name: \"3000\"\n    port: 3000"), "not be only digits"},
		{"duplicate", "version: 1\nports:\n  - name: a\n    port: 1\n  - name: a\n    port: 2\n", "ports[1] (a): duplicate name"},
		{"port range", entry("name: a\n    port: 70000"), "port must be between 1 and 65535"},
		{"missing port", entry("name: a"), "port must be between"},
		{"proto", entry("name: a\n    port: 1\n    proto: sctp"), "proto must be tcp or udp"},
		{"expect", entry("name: a\n    port: 1\n    expect: closed"), "expect must be free or open"},
		{"udp open", entry("name: a\n    port: 1\n    proto: udp\n    expect: open"), "only supported for tcp"},
		{"host on free", entry("name: a\n    port: 1\n    host: db"), "host only applies to expect: open"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(tt.yaml))
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want ErrInvalid mentioning %q", err, tt.want)
			}
		})
	}
}

func TestParseErrorHidesGoTypes(t *testing.T) {
	t.Parallel()
	_, err := Parse([]byte("version: 1\nports:\n  - name: web\n    port: 3000\n    exepct: open\n"))
	if err == nil || strings.Contains(err.Error(), "in type") || strings.Contains(err.Error(), "project.") {
		t.Errorf("error leaks Go type names: %v", err)
	}
}

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindAndLoad(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(deep); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), deep) {
		t.Errorf("no file: %v", err)
	}

	ymlPath := write(t, filepath.Join(root, "a"), ".portcheck.yml", example)
	if p, err := Find(deep); err != nil || p != ymlPath {
		t.Errorf("Find = %q, %v; want the .yml in a parent", p, err)
	}
	yamlPath := write(t, filepath.Join(root, "a"), ".portcheck.yaml", example)
	if p, _ := Find(deep); p != yamlPath {
		t.Errorf("Find = %q; .yaml must win over .yml", p)
	}
	// A directory with the config name is ignored.
	if err := os.Mkdir(filepath.Join(deep, ".portcheck.yaml"), 0o750); err != nil {
		t.Fatal(err)
	}
	if p, _ := Find(deep); p != yamlPath {
		t.Errorf("Find = %q; directories must be skipped", p)
	}

	cfg, err := Load(yamlPath)
	if err != nil || cfg.Path != yamlPath || len(cfg.Ports) != 4 {
		t.Fatalf("Load = %+v, %v", cfg, err)
	}
	bad := write(t, root, "bad.yaml", "version: 9\n")
	if _, err := Load(bad); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), bad) {
		t.Errorf("invalid file error = %v", err)
	}
	if _, err := Load(filepath.Join(root, "missing.yaml")); err == nil {
		t.Error("missing file must fail")
	}
}

func TestFindRelativeDirStops(t *testing.T) {
	t.Parallel()
	// A relative dir can't reach parents, but the search must still end.
	if _, err := Find("."); !errors.Is(err, ErrNotFound) {
		t.Errorf("Find(.) = %v, want ErrNotFound (no config in this package)", err)
	}
}

// TestSchemaMatchesStruct keeps docs/schema/portcheck.schema.json in sync with Port.
func TestSchemaMatchesStruct(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "schema", "portcheck.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		ID         string `json:"$id"`
		Properties struct {
			Ports struct {
				Items struct {
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"items"`
			} `json:"ports"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if schema.ID != SchemaURL {
		t.Errorf("$id = %q, want %q", schema.ID, SchemaURL)
	}
	typ := reflect.TypeFor[Port]()
	fields := make([]string, 0, typ.NumField())
	for i := range typ.NumField() {
		fields = append(fields, typ.Field(i).Tag.Get("yaml"))
	}
	props := make([]string, 0, len(schema.Properties.Ports.Items.Properties))
	for k := range schema.Properties.Ports.Items.Properties {
		props = append(props, k)
	}
	slices.Sort(fields)
	slices.Sort(props)
	if d := cmp.Diff(fields, props); d != "" {
		t.Errorf("schema properties differ from Port fields (-struct +schema):\n%s", d)
	}
}
