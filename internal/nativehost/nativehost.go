// Package nativehost implements portcheck's side of browser native messaging
// (ADR-0005): the message framing, the allowlist of operations a browser
// extension may request, and registering portcheck with browsers.
//
// A browser starts the host with the calling extension's origin as the first
// argument and exchanges JSON messages over stdin/stdout, each preceded by its
// length as a 32-bit unsigned integer in native byte order.
package nativehost

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// HostName is the name portcheck is registered under; extensions pass it to
// chrome.runtime.sendNativeMessage.
const HostName = "io.github.roystondwork_dev.portcheck"

const (
	// MaxIncoming is the largest message accepted from the browser. Browsers
	// send at most 64 MiB; requests are tiny, so anything larger is an error.
	MaxIncoming = 1 << 20
	// MaxOutgoing is the largest message browsers accept from a host (1 MiB).
	MaxOutgoing = 1 << 20
)

// IsLaunch reports whether args (without the program name) are those a
// browser uses to start a native messaging host. Chromium browsers pass the
// extension origin (followed on Windows by --parent-window); Firefox passes
// the path of the host manifest and the add-on ID.
func IsLaunch(args []string) bool {
	if len(args) > 0 && strings.HasPrefix(args[0], "chrome-extension://") {
		return true
	}
	if len(args) < 2 {
		return false
	}
	// The manifest path's last element, whichever separator the OS uses.
	name := args[0][strings.LastIndexAny(args[0], `/\`)+1:]
	return name == HostName+".json"
}

// ReadMessage reads one length-prefixed message. It returns io.EOF when the
// browser has closed the connection between messages.
func ReadMessage(r io.Reader) ([]byte, error) {
	var n uint32
	if err := binary.Read(r, binary.NativeEndian, &n); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("read message length: %w", err)
	}
	if n > MaxIncoming {
		return nil, fmt.Errorf("message of %d bytes is too large (limit %d)", n, MaxIncoming)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read message: %w", err)
	}
	return buf, nil
}

// WriteMessage writes one length-prefixed message.
func WriteMessage(w io.Writer, msg []byte) error {
	if len(msg) > MaxOutgoing {
		return fmt.Errorf("message of %d bytes is too large (limit %d)", len(msg), MaxOutgoing)
	}
	buf := make([]byte, 4+len(msg))
	binary.NativeEndian.PutUint32(buf, uint32(len(msg)))
	copy(buf[4:], msg)
	if _, err := w.Write(buf); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	return nil
}

// Request is a message from the extension.
type Request struct {
	// ID is echoed in the response so the extension can match replies.
	ID json.RawMessage `json:"id,omitempty"`
	// Op is one of "version", "list", "check" and "kill".
	Op string `json:"op"`
	// Port is the port for "check".
	Port int `json:"port,omitempty"`
	// PID is the process for "kill".
	PID int `json:"pid,omitempty"`
	// Force skips the graceful request for "kill".
	Force bool `json:"force,omitempty"`
}

// Response is a message to the extension.
type Response struct {
	ID json.RawMessage `json:"id,omitempty"`
	// OK is true when Data holds portcheck's JSON output.
	OK bool `json:"ok"`
	// ExitCode is portcheck's exit status for the operation (see docs/exit-codes.md).
	ExitCode int `json:"exitCode"`
	// Data is the schema v1 envelope the matching --json command prints.
	Data json.RawMessage `json:"data,omitempty"`
	// Error explains a failure.
	Error string `json:"error,omitempty"`
}

// Args maps a request to the portcheck command line that serves it. Only the
// operations in ADR-0005 are allowed, with validated arguments.
func Args(req Request) ([]string, error) {
	switch req.Op {
	case "version":
		return []string{"version", "--json"}, nil
	case "list":
		return []string{"list", "--listening", "--json"}, nil
	case "check":
		if req.Port < 1 || req.Port > 65535 {
			return nil, fmt.Errorf("check needs a port between 1 and 65535, got %d", req.Port)
		}
		return []string{"check", strconv.Itoa(req.Port), "--json"}, nil
	case "kill":
		if req.PID < 1 {
			return nil, fmt.Errorf("kill needs a positive pid, got %d", req.PID)
		}
		args := []string{"kill", "--pid", strconv.Itoa(req.PID), "--yes", "--json"}
		if req.Force {
			args = append(args, "--force")
		}
		return args, nil
	default:
		return nil, fmt.Errorf("unsupported operation %q (want version, list, check or kill)", req.Op)
	}
}

// RunFunc runs a portcheck command line and returns its stdout, stderr and
// exit code.
type RunFunc func(ctx context.Context, args []string) (stdout, stderr []byte, code int)

// Handle serves one request.
func Handle(ctx context.Context, raw []byte, run RunFunc) Response {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return Response{ExitCode: 2, Error: "invalid request: " + err.Error()}
	}
	args, err := Args(req)
	if err != nil {
		return Response{ID: req.ID, ExitCode: 2, Error: err.Error()}
	}
	stdout, stderr, code := run(ctx, args)
	resp := Response{ID: req.ID, ExitCode: code, Error: firstLine(stderr)}
	if out := strings.TrimSpace(string(stdout)); out != "" && json.Valid([]byte(out)) {
		resp.OK = true
		resp.Data = json.RawMessage(out)
	} else if resp.Error == "" {
		resp.Error = fmt.Sprintf("portcheck exited with status %d", code)
	}
	return resp
}

// Serve answers requests from r on w until the browser closes the connection.
func Serve(ctx context.Context, r io.Reader, w io.Writer, run RunFunc) error {
	for {
		raw, err := ReadMessage(r)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		resp := Handle(ctx, raw, run)
		// Can't fail: Data is checked with json.Valid and ID came from json.Unmarshal.
		out, _ := json.Marshal(resp)
		if len(out) > MaxOutgoing {
			out, _ = json.Marshal(Response{ID: resp.ID, ExitCode: resp.ExitCode, Error: "portcheck's answer is too large for the browser"})
		}
		if err := WriteMessage(w, out); err != nil {
			return err
		}
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
