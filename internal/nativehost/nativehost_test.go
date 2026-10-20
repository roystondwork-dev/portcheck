package nativehost

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func frame(msg string) []byte {
	var b bytes.Buffer
	if err := WriteMessage(&b, []byte(msg)); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func TestFraming(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	for _, m := range []string{`{"op":"list"}`, ``, `{"op":"check","port":3000}`} {
		if err := WriteMessage(&b, []byte(m)); err != nil {
			t.Fatal(err)
		}
	}
	if got := binary.NativeEndian.Uint32(b.Bytes()); got != 13 {
		t.Fatalf("length prefix = %d, want 13", got)
	}
	for _, want := range []string{`{"op":"list"}`, ``, `{"op":"check","port":3000}`} {
		got, err := ReadMessage(&b)
		if err != nil || string(got) != want {
			t.Fatalf("ReadMessage = %q, %v; want %q", got, err, want)
		}
	}
	if _, err := ReadMessage(&b); !errors.Is(err, io.EOF) {
		t.Fatalf("at end: %v, want io.EOF", err)
	}
}

func TestReadMessageErrors(t *testing.T) {
	t.Parallel()
	tooBig := binary.NativeEndian.AppendUint32(nil, MaxIncoming+1)
	cases := map[string][]byte{
		"truncated length": {1, 0},
		"truncated body":   append(binary.NativeEndian.AppendUint32(nil, 10), "abc"...),
		"too large":        tooBig,
	}
	for name, in := range cases {
		if _, err := ReadMessage(bytes.NewReader(in)); err == nil || errors.Is(err, io.EOF) {
			t.Errorf("%s: err = %v, want a non-EOF error", name, err)
		}
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestWriteMessageErrors(t *testing.T) {
	t.Parallel()
	if err := WriteMessage(io.Discard, make([]byte, MaxOutgoing+1)); err == nil {
		t.Error("oversized message accepted")
	}
	if err := WriteMessage(failWriter{}, []byte("x")); err == nil || !strings.Contains(err.Error(), "broken pipe") {
		t.Errorf("write error = %v", err)
	}
}

func TestIsLaunch(t *testing.T) {
	t.Parallel()
	for args, want := range map[string]bool{
		"chrome-extension://fmocabgaaihiflefcdafnhoihcibgnhl/":                   true,
		"chrome-extension://fmocabgaaihiflefcdafnhoihcibgnhl/ --parent-window=0": true,
		"list --listening": false,
		"":                 false,
	} {
		if got := IsLaunch(strings.Fields(args)); got != want {
			t.Errorf("IsLaunch(%q) = %v, want %v", args, got, want)
		}
	}
}

func TestArgs(t *testing.T) {
	t.Parallel()
	ok := map[string]struct {
		req  Request
		want []string
	}{
		"version":    {Request{Op: "version"}, []string{"version", "--json"}},
		"list":       {Request{Op: "list"}, []string{"list", "--listening", "--json"}},
		"check":      {Request{Op: "check", Port: 3000}, []string{"check", "3000", "--json"}},
		"kill":       {Request{Op: "kill", PID: 42}, []string{"kill", "--pid", "42", "--yes", "--json"}},
		"kill force": {Request{Op: "kill", PID: 42, Force: true}, []string{"kill", "--pid", "42", "--yes", "--json", "--force"}},
	}
	for name, tc := range ok {
		got, err := Args(tc.req)
		if err != nil || !cmp.Equal(got, tc.want) {
			t.Errorf("%s: Args = %q, %v; want %q", name, got, err, tc.want)
		}
	}
	for _, req := range []Request{
		{Op: "check"}, {Op: "check", Port: 70000}, {Op: "kill"}, {Op: "kill", PID: -1},
		{Op: "run"}, {Op: "scan"}, {Op: ""},
	} {
		if _, err := Args(req); err == nil {
			t.Errorf("Args(%+v) accepted", req)
		}
	}
}

func fakeRun(stdout, stderr string, code int, seen *[][]string) RunFunc {
	return func(_ context.Context, args []string) ([]byte, []byte, int) {
		if seen != nil {
			*seen = append(*seen, args)
		}
		return []byte(stdout), []byte(stderr), code
	}
}

func TestHandle(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	envelope := `{"schemaVersion":1,"kind":"check","data":{}}`
	var seen [][]string
	cases := []struct {
		name string
		raw  string
		run  RunFunc
		want Response
	}{
		{"ok", `{"id":7,"op":"check","port":3000}`, fakeRun(envelope+"\n", "", 1, &seen),
			Response{ID: json.RawMessage("7"), OK: true, ExitCode: 1, Data: json.RawMessage(envelope)}},
		{"string id and stderr note", `{"id":"a","op":"list"}`, fakeRun(envelope, "hint: re-run with sudo\n", 0, nil),
			Response{ID: json.RawMessage(`"a"`), OK: true, Data: json.RawMessage(envelope), Error: "hint: re-run with sudo"}},
		{"failure", `{"op":"kill","pid":5}`, fakeRun("", "\nportcheck: refusing to kill launchd (pid 5)\n", 1, nil),
			Response{ExitCode: 1, Error: "refusing to kill launchd (pid 5)"}},
		{"failure without message", `{"op":"version"}`, fakeRun("not json", "", 3, nil),
			Response{ExitCode: 3, Error: "portcheck exited with status 3"}},
		{"invalid json", `{"op":`, nil, Response{ExitCode: 2, Error: "invalid request: unexpected end of JSON input"}},
		{"not allowed", `{"id":1,"op":"run"}`, nil,
			Response{ID: json.RawMessage("1"), ExitCode: 2, Error: `unsupported operation "run" (want version, list, check or kill)`}},
	}
	for _, tc := range cases {
		if got := Handle(ctx, []byte(tc.raw), tc.run); !cmp.Equal(got, tc.want) {
			t.Errorf("%s: %s", tc.name, cmp.Diff(tc.want, got))
		}
	}
	if !cmp.Equal(seen, [][]string{{"check", "3000", "--json"}}) {
		t.Errorf("ran %q", seen)
	}
}

func TestServe(t *testing.T) {
	t.Parallel()
	in := bytes.NewReader(append(frame(`{"id":1,"op":"version"}`), frame(`{"id":2,"op":"nope"}`)...))
	var out bytes.Buffer
	if err := Serve(t.Context(), in, &out, fakeRun(`{"kind":"version"}`, "", 0, nil)); err != nil {
		t.Fatal(err)
	}
	var got []Response
	for {
		raw, err := ReadMessage(&out)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var r Response
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	want := []Response{
		{ID: json.RawMessage("1"), OK: true, Data: json.RawMessage(`{"kind":"version"}`)},
		{ID: json.RawMessage("2"), ExitCode: 2, Error: `unsupported operation "nope" (want version, list, check or kill)`},
	}
	if !cmp.Equal(got, want) {
		t.Error(cmp.Diff(want, got))
	}
}

func TestServeTooLargeAnswer(t *testing.T) {
	t.Parallel()
	big := `{"pad":"` + strings.Repeat("x", MaxOutgoing) + `"}`
	var out bytes.Buffer
	if err := Serve(t.Context(), bytes.NewReader(frame(`{"id":1,"op":"list"}`)), &out, fakeRun(big, "", 0, nil)); err != nil {
		t.Fatal(err)
	}
	raw, err := ReadMessage(&out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "too large for the browser") || !strings.Contains(string(raw), `"id":1`) {
		t.Errorf("response = %s", raw)
	}
}

func TestServeErrors(t *testing.T) {
	t.Parallel()
	if err := Serve(t.Context(), bytes.NewReader([]byte{9, 0}), io.Discard, nil); err == nil {
		t.Error("truncated input accepted")
	}
	if err := Serve(t.Context(), bytes.NewReader(frame(`{"op":"version"}`)), failWriter{}, fakeRun("{}", "", 0, nil)); err == nil {
		t.Error("write error ignored")
	}
}
