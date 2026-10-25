// Package mcpserver serves portcheck to AI assistants over the Model Context
// Protocol (ADR-0007). Each tool runs a portcheck command in-process with
// --json and returns its schema v1 output, so assistants see exactly what the
// CLI prints and kill keeps all its safety guards.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunFunc runs a portcheck command line and returns its stdout, stderr and
// exit code.
type RunFunc func(ctx context.Context, args []string) (stdout, stderr []byte, code int)

const instructions = `portcheck shows which local process holds a TCP/UDP port and frees it safely.
When a dev server, database or test fails with "address already in use" / EADDRINUSE,
call check_port with that port to see what holds it (name, PID, who started it), then
stop_process if the user wants it stopped, or find_free_port to use another port.
wait_for_port waits until a service accepts connections (or a port is free).
stop_process never stops system processes or the assistant itself, and can't stop
other users' processes without privileges; containers get a stop command instead.`

// ListInput filters list_ports.
type ListInput struct {
	Ports string `json:"ports,omitempty" jsonschema:"only these local ports, e.g. 3000 or 3000-3010,8080"`
	Name  string `json:"name,omitempty" jsonschema:"only processes whose name matches this glob, e.g. node*"`
	Proto string `json:"proto,omitempty" jsonschema:"tcp or udp (default both)"`
}

// CheckInput is check_port's input.
type CheckInput struct {
	Ports []int  `json:"ports" jsonschema:"the ports to check, e.g. [3000]"`
	Proto string `json:"proto,omitempty" jsonschema:"tcp (default) or udp"`
}

// FreeInput is find_free_port's input.
type FreeInput struct {
	Range string `json:"range,omitempty" jsonschema:"where to look, e.g. 3000-3999 (default: from 1024 up)"`
	Count int    `json:"count,omitempty" jsonschema:"how many free ports (default 1)"`
}

// StopInput is stop_process's input: exactly one of Port and PID.
type StopInput struct {
	Port   int  `json:"port,omitempty" jsonschema:"stop what listens on this port"`
	PID    int  `json:"pid,omitempty" jsonschema:"stop this process"`
	Force  bool `json:"force,omitempty" jsonschema:"kill at once instead of asking it to exit first"`
	DryRun bool `json:"dry_run,omitempty" jsonschema:"only report what would be stopped"`
}

// WaitInput is wait_for_port's input.
type WaitInput struct {
	Port           int    `json:"port" jsonschema:"the port to wait for"`
	Host           string `json:"host,omitempty" jsonschema:"host (default localhost)"`
	Free           bool   `json:"free,omitempty" jsonschema:"wait until the port is free instead of open"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"give up after this many seconds (default 30, at most 300)"`
}

func validPort(p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", p)
	}
	return nil
}

func validProto(p string) error {
	if p != "" && p != "tcp" && p != "udp" {
		return fmt.Errorf("proto must be tcp or udp, got %q", p)
	}
	return nil
}

// ListArgs maps list_ports to a command line.
func ListArgs(in ListInput) ([]string, error) {
	if err := validProto(in.Proto); err != nil {
		return nil, err
	}
	args := []string{"list", "--listening", "--json"}
	if in.Ports != "" {
		args = append(args, "--port", in.Ports)
	}
	if in.Name != "" {
		args = append(args, "--name", in.Name)
	}
	if in.Proto != "" {
		args = append(args, "--proto", in.Proto)
	}
	return args, nil
}

// CheckArgs maps check_port to a command line.
func CheckArgs(in CheckInput) ([]string, error) {
	if len(in.Ports) == 0 {
		return nil, errors.New("give at least one port")
	}
	if err := validProto(in.Proto); err != nil {
		return nil, err
	}
	args := []string{"check"}
	for _, p := range in.Ports {
		if err := validPort(p); err != nil {
			return nil, err
		}
		args = append(args, strconv.Itoa(p))
	}
	if in.Proto != "" {
		args = append(args, "--proto", in.Proto)
	}
	return append(args, "--json"), nil
}

// FreeArgs maps find_free_port to a command line.
func FreeArgs(in FreeInput) ([]string, error) {
	if in.Count < 0 || in.Count > 100 {
		return nil, fmt.Errorf("count must be between 1 and 100, got %d", in.Count)
	}
	args := []string{"free"}
	if in.Range != "" {
		args = append(args, in.Range)
	}
	if in.Count > 0 {
		args = append(args, "-n", strconv.Itoa(in.Count))
	}
	return append(args, "--json"), nil
}

// StopArgs maps stop_process to a command line. It never passes
// --allow-system: the safety guards always apply.
func StopArgs(in StopInput) ([]string, error) {
	if (in.Port == 0) == (in.PID == 0) {
		return nil, errors.New("give exactly one of port and pid")
	}
	args := []string{"kill"}
	if in.Port != 0 {
		if err := validPort(in.Port); err != nil {
			return nil, err
		}
		args = append(args, strconv.Itoa(in.Port))
	} else {
		if in.PID < 1 {
			return nil, fmt.Errorf("pid must be positive, got %d", in.PID)
		}
		args = append(args, "--pid", strconv.Itoa(in.PID))
	}
	args = append(args, "--yes", "--json")
	if in.Force {
		args = append(args, "--force")
	}
	if in.DryRun {
		args = append(args, "--dry-run")
	}
	return args, nil
}

// WaitArgs maps wait_for_port to a command line.
func WaitArgs(in WaitInput) ([]string, error) {
	if err := validPort(in.Port); err != nil {
		return nil, err
	}
	timeout := in.TimeoutSeconds
	if timeout == 0 {
		timeout = 30
	}
	if timeout < 1 || timeout > 300 {
		return nil, fmt.Errorf("timeout_seconds must be between 1 and 300, got %d", in.TimeoutSeconds)
	}
	target := strconv.Itoa(in.Port)
	if in.Host != "" {
		target = in.Host + ":" + target
	}
	args := []string{"wait", target, "--timeout", strconv.Itoa(timeout) + "s", "--json"}
	if in.Free {
		args = append(args, "--free")
	}
	return args, nil
}

// result runs args and turns portcheck's output into a tool result. Exit
// codes in ok are answers (e.g. 1 = "in use" for check), not errors.
func result(ctx context.Context, run RunFunc, args []string, ok ...int) *mcp.CallToolResult {
	stdout, stderr, code := run(ctx, args)
	out := strings.TrimSpace(string(stdout))
	if !slices.Contains(ok, code) || out == "" {
		msg := firstLine(stderr)
		if msg == "" {
			msg = out
		}
		if msg == "" {
			msg = fmt.Sprintf("portcheck exited with status %d", code)
		}
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
	}
	if json.Valid([]byte(out)) {
		out = fmt.Sprintf("exit code %d (%s)\n%s", code, meaning(args[0], code), out)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}
}

// meaning explains an exit code for the assistant (docs/exit-codes.md).
func meaning(cmd string, code int) string {
	switch {
	case code == 0 && cmd == "check":
		return "all free"
	case code == 1 && cmd == "check":
		return "at least one port is in use"
	case code == 1 && cmd == "free":
		return "no free port found"
	case code == 1 && cmd == "wait":
		return "timed out"
	case code == 1 && cmd == "kill":
		return "nothing was stopped"
	case code == 3:
		return "some processes couldn't be stopped"
	case code == 4:
		return "permission denied for some processes"
	default:
		return "ok"
	}
}

func firstLine(b []byte) string {
	for line := range strings.SplitSeq(string(b), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return strings.TrimPrefix(line, "portcheck: ")
		}
	}
	return ""
}

func boolPtr(b bool) *bool { return &b }

// tool registers a tool whose input maps to a command line with argsFn.
func tool[In any](s *mcp.Server, t *mcp.Tool, run RunFunc, argsFn func(In) ([]string, error), ok ...int) {
	mcp.AddTool(s, t, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		args, err := argsFn(in)
		if err != nil {
			return nil, nil, err // the SDK reports it to the assistant as a tool error
		}
		return result(ctx, run, args, ok...), nil, nil
	})
}

// checkSchema is check_port's input schema. Inferred from CheckInput, ports
// would be "array or null" (a nil Go slice); it's a required, non-empty array.
func checkSchema() *jsonschema.Schema {
	sc, err := jsonschema.For[CheckInput](nil)
	if err != nil {
		panic(err) // CheckInput is a fixed type: only a programming error gets here
	}
	ports := sc.Properties["ports"]
	ports.Types, ports.Type, ports.MinItems = nil, "array", new(int)
	*ports.MinItems = 1
	return sc
}

// New returns the MCP server; version is portcheck's.
func New(version string, run RunFunc) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "portcheck", Title: "portcheck", Version: version}, &mcp.ServerOptions{Instructions: instructions})
	// Looking things up changes nothing, so the read-only tools are also idempotent
	// and not destructive. All four hints are explicit on every tool: some
	// directories reject tools that leave any of them out.
	readOnly := func(title string) *mcp.ToolAnnotations {
		return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, DestructiveHint: boolPtr(false), IdempotentHint: true, OpenWorldHint: boolPtr(false)}
	}
	tool(s, &mcp.Tool{
		Name: "list_ports",
		Description: "List the ports listening on this machine, with the process, PID, user and who started it " +
			"(e.g. npm ← zsh), and the container for ports published by Docker or Podman. " +
			"Use it to see what's running, or to find a dev server by name (name=\"node*\") or port range.",
		Annotations: readOnly("List listening ports"),
	}, run, ListArgs, 0, 4)
	tool(s, &mcp.Tool{
		Name: "check_port",
		Description: "Check whether ports are free. For each port in use, says which process holds it: name, PID, user, " +
			"command line, who started it, and the container if Docker or Podman publishes it. " +
			"Use it first when a server fails with \"address already in use\" or EADDRINUSE.",
		InputSchema: checkSchema(),
		Annotations: readOnly("Check a port"),
	}, run, CheckArgs, 0, 1)
	tool(s, &mcp.Tool{
		Name: "find_free_port",
		Description: "Find ports that are free right now, verified by binding them, optionally within a range " +
			"(e.g. 3000-3999). Use it to start a server on another port instead of stopping what holds one.",
		Annotations: readOnly("Find free ports"),
	}, run, FreeArgs, 0, 1)
	tool(s, &mcp.Tool{
		Name: "stop_process",
		Description: "Stop the process listening on a port, or a process by PID: asks it to exit, then kills it after 5 s. " +
			"Refuses system processes and the assistant itself. Ports published by Docker, Podman or WSL report the command to run instead. " +
			"Use dry_run to see what would be stopped, and check_port first to tell the user what it is.",
		Annotations: &mcp.ToolAnnotations{Title: "Stop a process", DestructiveHint: boolPtr(true), OpenWorldHint: boolPtr(false)},
	}, run, StopArgs, 0, 1, 3, 4)
	tool(s, &mcp.Tool{
		Name: "wait_for_port",
		Description: "Wait until a port accepts connections, e.g. until a server or database has finished starting, " +
			"or with free=true until a port is free again. Returns as soon as it's ready, or fails after timeout_seconds.",
		Annotations: readOnly("Wait for a port"),
	}, run, WaitArgs, 0, 1)
	return s
}
