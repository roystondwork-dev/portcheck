package output

import (
	"strings"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/monitor"
)

func TestMonitorsDoc(t *testing.T) {
	t.Parallel()
	d := MonitorsDoc("/cfg/monitors.yaml", []monitor.Target{
		{Name: "site", URL: "https://example.com/health", Every: "30s", Contains: "ok"},
		{Name: "api", URL: "https://api.example.com/", Status: "200-399"},
		{Name: "db", Address: "db:5432", Every: "2h"},
	})
	got := string(render(t, d, Plain, Options{}))
	for _, want := range []string{
		"site", "https://example.com/health", "30s", `HTTP 200-299, contains "ok"`,
		"api", "1m", "HTTP 200-399",
		"db", "db:5432", "2h", "accepts connections",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if js := string(render(t, d, JSON, Options{})); !strings.Contains(js, `"file": "/cfg/monitors.yaml"`) || !strings.Contains(js, `"kind": "monitors"`) {
		t.Errorf("json:\n%s", js)
	}
	if empty := string(render(t, MonitorsDoc("p", nil), Plain, Options{})); !strings.Contains(empty, "No monitors yet") {
		t.Errorf("empty: %s", empty)
	}
}

func TestMonitorCheckDoc(t *testing.T) {
	t.Parallel()
	d := MonitorCheckDoc([]monitor.Result{
		{Name: "site", Up: true, Status: 200, Latency: 84.6},
		{Name: "db", Up: true, Latency: 1.2},
		{Name: "api", Error: "HTTP 503 Service Unavailable", Status: 503, Latency: 40},
	})
	got := string(render(t, d, Plain, Options{}))
	for _, want := range []string{"site", "up", "85 ms", "HTTP 200", "db", "1 ms", "api", "down", "HTTP 503 Service Unavailable"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if js := string(render(t, d, JSON, Options{})); !strings.Contains(js, `"kind": "monitorCheck"`) {
		t.Errorf("json:\n%s", js)
	}
}

func TestShortDuration(t *testing.T) {
	t.Parallel()
	for d, want := range map[time.Duration]string{
		30 * time.Second:             "30s",
		time.Minute:                  "1m",
		90 * time.Second:             "1m30s",
		2 * time.Hour:                "2h",
		2*time.Hour + 30*time.Minute: "2h30m",
		time.Hour + 30*time.Second:   "1h0m30s",
	} {
		if got := ShortDuration(d); got != want {
			t.Errorf("ShortDuration(%s) = %q, want %q", d, got, want)
		}
	}
}
