package mcpserver

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		got     func() ([]string, error)
		want    []string
		wantErr string
	}{
		{"list", func() ([]string, error) { return ListArgs(ListInput{}) }, []string{"list", "--listening", "--json"}, ""},
		{"list filtered", func() ([]string, error) { return ListArgs(ListInput{Ports: "3000-3010", Name: "node*", Proto: "tcp"}) },
			[]string{"list", "--listening", "--json", "--port", "3000-3010", "--name", "node*", "--proto", "tcp"}, ""},
		{"list bad proto", func() ([]string, error) { return ListArgs(ListInput{Proto: "sctp"}) }, nil, `proto must be tcp or udp, got "sctp"`},
		{"check", func() ([]string, error) { return CheckArgs(CheckInput{Ports: []int{3000, 8080}}) }, []string{"check", "3000", "8080", "--json"}, ""},
		{"check udp", func() ([]string, error) { return CheckArgs(CheckInput{Ports: []int{53}, Proto: "udp"}) }, []string{"check", "53", "--proto", "udp", "--json"}, ""},
		{"check none", func() ([]string, error) { return CheckArgs(CheckInput{}) }, nil, "give at least one port"},
		{"check bad port", func() ([]string, error) { return CheckArgs(CheckInput{Ports: []int{70000}}) }, nil, "port must be between 1 and 65535, got 70000"},
		{"check bad proto", func() ([]string, error) { return CheckArgs(CheckInput{Ports: []int{1}, Proto: "x"}) }, nil, `proto must be tcp or udp, got "x"`},
		{"free", func() ([]string, error) { return FreeArgs(FreeInput{}) }, []string{"free", "--json"}, ""},
		{"free range", func() ([]string, error) { return FreeArgs(FreeInput{Range: "3000-3999", Count: 3}) }, []string{"free", "3000-3999", "-n", "3", "--json"}, ""},
		{"free bad count", func() ([]string, error) { return FreeArgs(FreeInput{Count: 101}) }, nil, "count must be between 1 and 100, got 101"},
		{"stop port", func() ([]string, error) { return StopArgs(StopInput{Port: 3000}) }, []string{"kill", "3000", "--yes", "--json"}, ""},
		{"stop pid forced dry", func() ([]string, error) { return StopArgs(StopInput{PID: 42, Force: true, DryRun: true}) },
			[]string{"kill", "--pid", "42", "--yes", "--json", "--force", "--dry-run"}, ""},
		{"stop both", func() ([]string, error) { return StopArgs(StopInput{Port: 1, PID: 1}) }, nil, "give exactly one of port and pid"},
		{"stop neither", func() ([]string, error) { return StopArgs(StopInput{}) }, nil, "give exactly one of port and pid"},
		{"stop bad port", func() ([]string, error) { return StopArgs(StopInput{Port: -1}) }, nil, "port must be between 1 and 65535, got -1"},
		{"stop bad pid", func() ([]string, error) { return StopArgs(StopInput{PID: -5}) }, nil, "pid must be positive, got -5"},
		{"wait", func() ([]string, error) { return WaitArgs(WaitInput{Port: 5432}) }, []string{"wait", "5432", "--timeout", "30s", "--json"}, ""},
		{"wait host free", func() ([]string, error) {
			return WaitArgs(WaitInput{Port: 3000, Host: "db", Free: true, TimeoutSeconds: 5})
		},
			[]string{"wait", "db:3000", "--timeout", "5s", "--json", "--free"}, ""},
		{"wait bad port", func() ([]string, error) { return WaitArgs(WaitInput{}) }, nil, "port must be between 1 and 65535, got 0"},
		{"wait bad timeout", func() ([]string, error) { return WaitArgs(WaitInput{Port: 1, TimeoutSeconds: 301}) }, nil, "timeout_seconds must be between 1 and 300, got 301"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.got()
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("args (-want +got):\n%s", diff)
			}
			// Never let an assistant switch off the safety guards.
			if slices.Contains(got, "--allow-system") {
				t.Errorf("args contain --allow-system: %v", got)
			}
		})
	}
}

func TestMeaning(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		cmd  string
		code int
		want string
	}{
		{"check", 0, "all free"}, {"check", 1, "at least one port is in use"}, {"free", 1, "no free port found"},
		{"wait", 1, "timed out"}, {"kill", 1, "nothing was stopped"}, {"kill", 3, "some processes couldn't be stopped"},
		{"kill", 4, "permission denied for some processes"}, {"list", 0, "ok"},
	} {
		if got := meaning(tt.cmd, tt.code); got != tt.want {
			t.Errorf("meaning(%s, %d) = %q, want %q", tt.cmd, tt.code, got, tt.want)
		}
	}
}

// connect starts the server with a fake portcheck and returns a client session.
func connect(t *testing.T, run RunFunc) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := New("v1.2.3", run).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

func TestServer(t *testing.T) {
	t.Parallel()
	var calls [][]string
	run := func(_ context.Context, args []string) ([]byte, []byte, int) {
		calls = append(calls, args)
		switch args[0] {
		case "check":
			return []byte(`{"schemaVersion":1,"kind":"check","data":{"ports":[]}}` + "\n"), nil, 1
		case "kill":
			return []byte("Nothing is listening on port 3000.\n"), nil, 1
		case "free":
			return nil, []byte("\nportcheck: invalid port range \"x\"\nRun 'portcheck free --help'\n"), 2
		case "wait":
			return []byte("not json"), nil, 9
		default:
			return nil, nil, 3
		}
	}
	cs := connect(t, run)

	init := cs.InitializeResult()
	if init.ServerInfo.Name != "portcheck" || init.ServerInfo.Version != "v1.2.3" || !strings.Contains(init.Instructions, "EADDRINUSE") {
		t.Errorf("initialize = %+v", init)
	}
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		destructive := tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint
		if destructive != (tool.Name == "stop_process") || tool.Annotations.ReadOnlyHint == (tool.Name == "stop_process") {
			t.Errorf("%s annotations = %+v", tool.Name, tool.Annotations)
		}
		// Directories score tools on these: every tool has a title and all four
		// hints, and the read-only ones are idempotent.
		if tool.Annotations.Title == "" || tool.Annotations.IdempotentHint != tool.Annotations.ReadOnlyHint ||
			tool.Annotations.DestructiveHint == nil || tool.Annotations.OpenWorldHint == nil {
			t.Errorf("%s annotations = %+v", tool.Name, tool.Annotations)
		}
		if tool.Name == "check_port" {
			b, _ := json.Marshal(tool.InputSchema)
			if want := `"ports":{"description":"the ports to check, e.g. [3000]","items":{"type":"integer"},"minItems":1,"type":"array"}`; !strings.Contains(string(b), want) {
				t.Errorf("check_port schema = %s, want it to contain %s", b, want)
			}
		}
	}
	slices.Sort(names)
	if diff := cmp.Diff([]string{"check_port", "find_free_port", "list_ports", "stop_process", "wait_for_port"}, names); diff != "" {
		t.Errorf("tools (-want +got):\n%s", diff)
	}

	text, isErr := call(t, cs, "check_port", map[string]any{"ports": []int{3000}})
	if isErr || !strings.HasPrefix(text, "exit code 1 (at least one port is in use)\n{") {
		t.Errorf("check_port = %q (error %v)", text, isErr)
	}
	text, isErr = call(t, cs, "stop_process", map[string]any{"port": 3000})
	if isErr || text != "Nothing is listening on port 3000." {
		t.Errorf("stop_process = %q (error %v)", text, isErr)
	}
	text, isErr = call(t, cs, "find_free_port", map[string]any{"range": "x"})
	if !isErr || text != `invalid port range "x"` {
		t.Errorf("find_free_port = %q (error %v)", text, isErr)
	}
	text, isErr = call(t, cs, "wait_for_port", map[string]any{"port": 1})
	if !isErr || text != "not json" {
		t.Errorf("wait_for_port = %q (error %v)", text, isErr)
	}
	text, isErr = call(t, cs, "list_ports", map[string]any{})
	if !isErr || text != "portcheck exited with status 3" {
		t.Errorf("list_ports = %q (error %v)", text, isErr)
	}
	text, isErr = call(t, cs, "stop_process", map[string]any{})
	if !isErr || text != "give exactly one of port and pid" {
		t.Errorf("stop_process {} = %q (error %v)", text, isErr)
	}
	if len(calls) != 5 {
		t.Errorf("portcheck ran %d times, want 5 (invalid input must not run it): %v", len(calls), calls)
	}
}
