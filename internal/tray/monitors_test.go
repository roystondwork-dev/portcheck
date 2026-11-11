package tray

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/monitor"
)

func TestMonitorViews(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 11, 11, 10, 42, 0, 0, time.UTC)
	targets := []monitor.Target{
		{Name: "site", URL: "https://example.com/"},
		{Name: "api", URL: "https://api.example.com/ping"},
		{Name: "db", Address: "db:5432"},
	}
	views := MonitorViews(targets, map[string]monitor.State{
		"site": {Status: monitor.Up, Since: since},
		"api":  {Status: monitor.Down, Since: since, Last: monitor.Result{Error: "timed out"}},
		"gone": {Status: monitor.Up},
	})
	want := []MonitorView{
		{Name: "site", Endpoint: "https://example.com/", IsURL: true, Status: monitor.Up, Since: since},
		{Name: "api", Endpoint: "https://api.example.com/ping", IsURL: true, Status: monitor.Down, Error: "timed out", Since: since},
		{Name: "db", Endpoint: "db:5432", Status: monitor.Unknown},
	}
	if !slices.Equal(views, want) {
		t.Errorf("views = %+v", views)
	}
	labels := []string{views[0].Label(), views[1].Label(), views[2].Label()}
	if !slices.Equal(labels, []string{"✓ site — up", "✗ api — down: timed out", "… db — checking"}) {
		t.Errorf("labels = %q", labels)
	}
	if s := monitorSummary(views); s != "Monitors: 1 up, 1 down, 1 checking" {
		t.Errorf("summary = %q", s)
	}
	if s := monitorSummary(views[:1]); s != "Monitors: 1 up" {
		t.Errorf("summary = %q", s)
	}
	if s := monitorSummary(nil); !strings.Contains(s, "portcheck monitor add") {
		t.Errorf("summary = %q", s)
	}
}

func TestMenuMonitors(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 11, 11, 10, 42, 0, 0, time.Local)
	var mu sync.Mutex
	views := []MonitorView{
		{Name: "site", Endpoint: "https://example.com/", IsURL: true, Status: monitor.Up, Since: since},
		{Name: "db", Endpoint: "db:5432", Status: monitor.Unknown},
	}
	w := &world{}
	d := w.deps()
	d.Monitors = func() []MonitorView { mu.Lock(); defer mu.Unlock(); return slices.Clone(views) }
	m := &fakeMenu{}
	tr := New(d, m)
	ctx := context.Background()
	tr.Refresh(ctx)
	lines := m.lines()
	for _, want := range []string{
		"Monitors: 1 up, 1 checking (off)",
		"✓ site — up",
		"  Open https://example.com/",
		"  Copy https://example.com/",
		"  Up since Wed 11 Nov 10:42 (off)",
		"… db — checking",
		"  Copy db:5432",
	} {
		if !slices.Contains(lines, want) {
			t.Errorf("menu missing %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	if slices.Contains(lines, "  Open db:5432") {
		t.Error("an address has an Open item")
	}
	m.clickOn(t, "  Open https://example.com/"[2:])
	m.clickOn(t, "Copy db:5432")
	if !slices.Equal(w.opened, []string{"https://example.com/"}) || !slices.Equal(w.copied, []string{"db:5432"}) {
		t.Errorf("opened %v copied %v", w.opened, w.copied)
	}

	// The site goes down: the title and summary say so, and the menu is redrawn.
	resets := m.resets
	mu.Lock()
	views[0] = MonitorView{Name: "site", Endpoint: "https://example.com/", IsURL: true, Status: monitor.Down, Error: "HTTP 503 Service Unavailable", Since: since}
	mu.Unlock()
	tr.Refresh(ctx)
	if m.resets != resets+1 || m.title != "⚠1" || !strings.Contains(m.tooltip, "No ports listening; 1 monitored service down") {
		t.Errorf("resets %d title %q tooltip %q", m.resets-resets, m.title, m.tooltip)
	}
	if !slices.Contains(m.lines(), "✗ site — down: HTTP 503 Service Unavailable") {
		t.Errorf("menu:\n%s", strings.Join(m.lines(), "\n"))
	}
	// Unchanged: no redraw.
	tr.Refresh(ctx)
	if m.resets != resets+1 {
		t.Error("redrawn without a change")
	}
	mu.Lock()
	views[1].Status, views[1].Error = monitor.Down, "connection refused"
	mu.Unlock()
	tr.Refresh(ctx)
	if !strings.Contains(m.tooltip, "2 monitored services down") || m.title != "⚠2" {
		t.Errorf("title %q tooltip %q", m.title, m.tooltip)
	}
}

func TestMenuWithoutMonitors(t *testing.T) {
	t.Parallel()
	w := &world{}
	m := &fakeMenu{}
	d := w.deps()
	d.Monitors = func() []MonitorView { return nil }
	New(d, m).Refresh(context.Background())
	if !slices.Contains(m.lines(), "Monitors: none (add one: portcheck monitor add <url>) (off)") {
		t.Errorf("menu:\n%s", strings.Join(m.lines(), "\n"))
	}
}
