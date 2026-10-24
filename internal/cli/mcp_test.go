package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

// TestMCP drives `portcheck mcp` over stdin/stdout like an AI assistant does,
// keeping stdin open until each answer arrives; the tools run portcheck
// in-process against the harness's fakes.
func TestMCP(t *testing.T) {
	t.Parallel()
	h := newHarness()
	h.finder.statuses = []freeport.Status{{Port: 3000, Proto: inspect.TCP, Availability: freeport.InUse,
		Holders: []inspect.Socket{{RawSocket: nodeSock, Process: &inspect.ProcessInfo{PID: 100, Name: "node"}}}}}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	h.app.Stdin, h.app.Stdout = inR, outW
	done := make(chan int, 1)
	go func() {
		code := h.run(t, "mcp")
		_ = outW.Close()
		done <- code
	}()
	lines := bufio.NewScanner(outR)
	ask := func(id int, method, params string) string {
		t.Helper()
		fmt.Fprintf(inW, `{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`+"\n", id, method, params)
		for lines.Scan() {
			var msg struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(lines.Bytes(), &msg); err == nil && msg.ID == id {
				return string(msg.Result)
			}
		}
		t.Fatalf("no answer to %s", method)
		return ""
	}

	if r := ask(1, "initialize", `{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}`); !strings.Contains(r, `"name":"portcheck"`) || !strings.Contains(r, `"version":"v1.2.3"`) {
		t.Errorf("initialize = %s", r)
	}
	fmt.Fprintln(inW, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if r := ask(2, "tools/call", `{"name":"check_port","arguments":{"ports":[3000]}}`); !strings.Contains(r, `exit code 1 (at least one port is in use)`) || !strings.Contains(r, `\"node\"`) {
		t.Errorf("check_port = %s", r)
	}
	if r := ask(3, "tools/call", `{"name":"stop_process","arguments":{"pid":100}}`); !strings.Contains(r, `\"terminated\"`) || strings.Contains(r, `"isError":true`) {
		t.Errorf("stop_process = %s", r)
	}
	_ = inW.Close() // the assistant quits
	if code := <-done; code != ExitOK {
		t.Errorf("exit %d: %s", code, h.stderr)
	}
}
