//go:build windows

package containers

import (
	"net/http"
	"testing"

	"github.com/Microsoft/go-winio"
)

func TestNamedPipe(t *testing.T) {
	t.Parallel()
	const pipe = `\\.\pipe\portcheck-test-engine`
	l, err := winio.ListenPipe(pipe, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: okHandler(t)}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })

	c, err := New("npipe:////./pipe/portcheck-test-engine")
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Published(t.Context())
	if err != nil || len(got) != len(wantPublished) {
		t.Fatalf("Published over named pipe = %v, %v", got, err)
	}
}
