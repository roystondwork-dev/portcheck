package tray

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
)

// fakeMenu records the menu as text lines ("  " per level, "-" for
// separators, "[x]"/"[ ]" for checkboxes, "(off)" for disabled items) and
// lets tests click items by title.
type fakeMenu struct {
	mu      sync.Mutex
	items   []*fakeItem
	title   string
	tooltip string
	resets  int
	quit    bool
}

type fakeItem struct {
	m       *fakeMenu
	depth   int
	title   string
	off     bool
	click   func()
	box     string
	tooltip string
}

func (m *fakeMenu) Reset() { m.mu.Lock(); m.items = nil; m.resets++; m.mu.Unlock() }
func (m *fakeMenu) add(depth int, title, tooltip, box string) *fakeItem {
	it := &fakeItem{m: m, depth: depth, title: title, tooltip: tooltip, box: box}
	m.items = append(m.items, it)
	return it
}
func (m *fakeMenu) Add(title, tooltip string) Item { return m.add(0, title, tooltip, "") }
func (m *fakeMenu) AddCheckbox(title string, checked bool) Item {
	box := "[ ] "
	if checked {
		box = "[x] "
	}
	return m.add(0, title, "", box)
}
func (m *fakeMenu) Separator()                     { m.add(0, "-", "", "") }
func (m *fakeMenu) SetTitle(title, tooltip string) { m.title, m.tooltip = title, tooltip }
func (m *fakeMenu) Quit()                          { m.quit = true }
func (i *fakeItem) Sub(title, tooltip string) Item { return i.m.add(i.depth+1, title, tooltip, "") }
func (i *fakeItem) Disable()                       { i.off = true }
func (i *fakeItem) OnClick(f func())               { i.click = f }

func (m *fakeMenu) lines() []string {
	out := make([]string, 0, len(m.items))
	for _, it := range m.items {
		l := strings.Repeat("  ", it.depth) + it.box + it.title
		if it.off {
			l += " (off)"
		}
		out = append(out, l)
	}
	return out
}

func (m *fakeMenu) clickOn(t *testing.T, title string) {
	t.Helper()
	for _, it := range m.items {
		if it.title == title && it.click != nil {
			it.click()
			return
		}
	}
	t.Fatalf("no clickable %q in:\n%s", title, strings.Join(m.lines(), "\n"))
}

var (
	listen = func(port uint16, pid int32) inspect.RawSocket {
		return inspect.RawSocket{Proto: inspect.TCP, Local: netip.AddrPortFrom(netip.IPv4Unspecified(), port), State: inspect.StateListen, PID: pid}
	}
	node = inspect.Socket{RawSocket: listen(3000, 42), Process: &inspect.ProcessInfo{
		PID: 42, Name: "node", User: "alice", Cmdline: "node ./node_modules/.bin/next dev --turbo --hostname 127.0.0.1 --port 3000 --experimental-https",
		Ancestors: []inspect.ProcessRef{{PID: 7, Name: "npm"}, {PID: 3, Name: "zsh"}},
	}}
	postgres = inspect.Socket{RawSocket: listen(5432, 99), Process: &inspect.ProcessInfo{PID: 99, Name: "postgres", User: "postgres"}}
	web      = inspect.Socket{RawSocket: listen(8080, 0), Container: &inspect.Container{ID: "abc", Name: "web", Runtime: "docker"}}
	hidden   = inspect.Socket{RawSocket: listen(22, 0)}
	udp      = inspect.Socket{RawSocket: inspect.RawSocket{Proto: inspect.UDP, Local: netip.AddrPortFrom(netip.IPv4Unspecified(), 53), PID: 5}}
	client   = inspect.Socket{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: netip.AddrPortFrom(netip.IPv4Unspecified(), 50000), State: inspect.StateEstablished, PID: 42}, Process: node.Process}
	plain    = inspect.Socket{RawSocket: listen(9000, 77), Process: &inspect.ProcessInfo{PID: 77, Name: "go", User: "alice"}}
	nameless = inspect.Socket{RawSocket: listen(9001, 78)}
)

func TestEntries(t *testing.T) {
	t.Parallel()
	all := make([]inspect.Socket, 0, 8)
	all = append(all, node, node, postgres, web, hidden, udp, client)
	got := Entries(all, "alice", false)
	want := []Entry{
		{Port: 3000, Name: "node", PID: 42, StartedBy: "npm (7) ← zsh (3)", Cmdline: node.Process.Cmdline},
		{Port: 8080, Name: "docker:web", StopHint: "docker stop web"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mine (-want +got):\n%s", diff)
	}
	everything := Entries(append(all, nameless), "alice", true)
	ports := make([]uint16, 0, len(everything))
	for _, e := range everything {
		ports = append(ports, e.Port)
	}
	if diff := cmp.Diff([]uint16{22, 3000, 5432, 8080, 9001}, ports); diff != "" {
		t.Errorf("all ports (-want +got):\n%s", diff)
	}
	if e := everything[0]; !e.Hidden || e.Name != "(hidden)" {
		t.Errorf("hidden = %+v", e)
	}
	if e := everything[4]; e.Name != "?" || e.PID != 78 {
		t.Errorf("nameless = %+v", e)
	}
	img := inspect.Socket{RawSocket: listen(6379, 0), Container: &inspect.Container{Image: "redis", Runtime: "podman"}}
	anon := inspect.Socket{RawSocket: listen(6380, 0), Container: &inspect.Container{Runtime: "docker"}}
	if got := Entries([]inspect.Socket{img, anon}, "", false); got[0].Name != "podman:redis" || got[1].Name != "docker:?" {
		t.Errorf("container names = %+v", got)
	}
	if e := (Entry{Port: 3000, Name: "node"}); e.Label() != ":3000  node" || e.URL() != "http://localhost:3000/" {
		t.Errorf("label/url = %q %q", e.Label(), e.URL())
	}
}

type world struct {
	mu      sync.Mutex
	sockets []inspect.Socket
	listErr error
	killed  []int32
	killErr error
	opened  []string
	copied  []string
	copyErr error
}

func (w *world) deps() Deps {
	return Deps{
		List: func(context.Context) ([]inspect.Socket, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			return w.sockets, w.listErr
		},
		Kill: func(_ context.Context, pid int32) (killer.Result, error) {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.killed = append(w.killed, pid)
			if w.killErr != nil {
				return killer.Result{PID: pid}, w.killErr
			}
			w.sockets = slicesDelete(w.sockets, pid)
			return killer.Result{PID: pid, Name: "node", Outcome: killer.Terminated}, nil
		},
		Open: func(url string) error { w.opened = append(w.opened, url); return nil },
		Copy: func(s string) error { w.copied = append(w.copied, s); return w.copyErr },
		User: "alice", Every: time.Millisecond,
	}
}

func slicesDelete(s []inspect.Socket, pid int32) []inspect.Socket {
	var out []inspect.Socket
	for _, x := range s {
		if x.PID != pid {
			out = append(out, x)
		}
	}
	return out
}

func TestMenu(t *testing.T) {
	t.Parallel()
	w := &world{sockets: []inspect.Socket{node, web, postgres, hidden}}
	m := &fakeMenu{}
	tr := New(w.deps(), m)
	ctx := context.Background()
	tr.Refresh(ctx)
	short := node.Process.Cmdline[:57] + "..."
	want := []string{
		"2 ports listening (off)",
		"-",
		":3000  node",
		"  Stop node (pid 42)",
		"  Open http://localhost:3000/",
		"  Copy PID",
		"  Started by npm (7) ← zsh (3) (off)",
		"  " + short,
		":8080  docker:web",
		"  Copy Stop Command",
		"  Open http://localhost:8080/",
		"-",
		"[ ] Show All Ports (also other users' and system)",
		"Refresh",
		"Quit portcheck",
	}
	if diff := cmp.Diff(want, m.lines()); diff != "" {
		t.Errorf("menu (-want +got):\n%s", diff)
	}
	if m.title != "2" || m.tooltip != "portcheck: 2 ports listening" {
		t.Errorf("title %q, tooltip %q", m.title, m.tooltip)
	}

	// Nothing changed: no redraw.
	tr.Refresh(ctx)
	if m.resets != 1 {
		t.Errorf("redrew an unchanged menu: %d resets", m.resets)
	}

	m.clickOn(t, "Copy PID")
	m.clickOn(t, "Copy Stop Command")
	m.clickOn(t, "Open http://localhost:3000/")
	m.clickOn(t, short)
	if diff := cmp.Diff([]string{"42", "docker stop web", node.Process.Cmdline}, w.copied); diff != "" {
		t.Errorf("copied (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"http://localhost:3000/"}, w.opened); diff != "" {
		t.Errorf("opened (-want +got):\n%s", diff)
	}
	if !strings.HasSuffix(m.tooltip, "Copied: "+node.Process.Cmdline) || m.resets != 1 {
		t.Errorf("tooltip %q after copying, %d resets", m.tooltip, m.resets)
	}

	// Stopping redraws without the port, with the result in the tooltip.
	m.clickOn(t, "Stop node (pid 42)")
	if diff := cmp.Diff([]int32{42}, w.killed); diff != "" {
		t.Errorf("killed (-want +got):\n%s", diff)
	}
	if m.title != "1" || !strings.Contains(m.tooltip, "node (pid 42): terminated") {
		t.Errorf("after stop: title %q, tooltip %q", m.title, m.tooltip)
	}

	// Show all ports.
	m.clickOn(t, "[ ] Show All Ports (also other users' and system)"[4:])
	lines := strings.Join(m.lines(), "\n")
	for _, s := range []string{":22  (hidden)", "  Owned by another user (off)", ":5432  postgres", "[x] Show All Ports"} {
		if !strings.Contains(lines, s) {
			t.Errorf("show all: missing %q in\n%s", s, lines)
		}
	}
	m.clickOn(t, "Quit portcheck")
	if !m.quit {
		t.Error("Quit didn't quit")
	}
}

func TestMenuErrorsAndEmpty(t *testing.T) {
	t.Parallel()
	w := &world{listErr: errors.New("permission denied")}
	m := &fakeMenu{}
	tr := New(w.deps(), m)
	ctx := context.Background()
	tr.Refresh(ctx)
	if m.lines()[0] != "permission denied (off)" || m.title != "!" {
		t.Errorf("error menu: %v, title %q", m.lines(), m.title)
	}
	w.listErr = nil
	m.clickOn(t, "Refresh")
	if m.lines()[0] != "No ports listening (off)" || m.title != "" {
		t.Errorf("empty menu: %v, title %q", m.lines(), m.title)
	}

	w.sockets = []inspect.Socket{node}
	w.killErr = errors.New("refusing to stop a protected system process")
	w.copyErr = errors.New("no clipboard")
	tr.Refresh(ctx)
	if m.lines()[0] != "1 port listening (off)" || m.title != "1" {
		t.Errorf("one port: %v, title %q", m.lines(), m.title)
	}
	m.clickOn(t, "Stop node (pid 42)")
	if !strings.Contains(m.tooltip, "couldn't stop node (pid 42): refusing") {
		t.Errorf("tooltip after a refused stop = %q", m.tooltip)
	}
	m.clickOn(t, "Copy PID")
	if !strings.HasSuffix(m.tooltip, "no clipboard") {
		t.Errorf("tooltip after a failed copy = %q", m.tooltip)
	}
	_ = plain
}

func TestRun(t *testing.T) {
	t.Parallel()
	w := &world{}
	m := &fakeMenu{}
	tr := New(w.deps(), m)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { tr.Run(ctx); close(done) }()
	time.Sleep(20 * time.Millisecond)
	w.mu.Lock()
	w.sockets = []inspect.Socket{plain}
	w.mu.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for {
		tr.mu.Lock()
		n := len(tr.entries)
		tr.mu.Unlock()
		if n == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.entries) != 1 || tr.entries[0].Port != 9000 {
		t.Errorf("Run didn't pick up the new port: %+v", tr.entries)
	}
}

func TestForgottenServers(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.Local)
	srv := func(port uint16, pid int32, project string, age time.Duration) inspect.Socket {
		return inspect.Socket{RawSocket: listen(port, pid), Process: &inspect.ProcessInfo{
			PID: pid, Name: "node", User: "alice", Project: project, CreateTime: now.Add(-age),
		}}
	}
	old := srv(5173, 10, "shop", 50*time.Hour)
	older := srv(3000, 11, "api", 72*time.Hour)
	older6 := older
	older6.Local = netip.AddrPortFrom(netip.IPv6Unspecified(), 3000)
	fresh := srv(4000, 12, "docs", time.Hour)
	noProject := srv(9000, 13, "", 100*time.Hour)

	w := &world{sockets: []inspect.Socket{old, older, older6, fresh, noProject}}
	d := w.deps()
	d.Now = func() time.Time { return now }
	m := &fakeMenu{}
	tr := New(d, m)
	ctx := context.Background()
	tr.Refresh(ctx)
	lines := m.lines()
	want := []string{
		"4 ports listening (off)",
		"2 dev servers running over a day",
		"  Stop All 2",
		"  :3000  node · api  (since " + older.Process.CreateTime.Format("Mon 2 Jan") + ")",
		"  :5173  node · shop  (since " + old.Process.CreateTime.Format("Mon 2 Jan") + ")",
		"-",
	}
	if diff := cmp.Diff(want, lines[:len(want)]); diff != "" {
		t.Errorf("menu top (-want +got):\n%s", diff)
	}
	if !slices.Contains(lines, "  Running since "+fresh.Process.CreateTime.Format("Mon 2 Jan 15:04")+" (off)") {
		t.Errorf("no start time for docs:\n%s", strings.Join(lines, "\n"))
	}
	if !slices.Contains(lines, ":4000  node · docs") {
		t.Errorf("no project in label:\n%s", strings.Join(lines, "\n"))
	}

	// One server: singular wording, and clicking a single server stops just it.
	m.clickOn(t, ":3000  node · api  (since "+older.Process.CreateTime.Format("Mon 2 Jan")+")")
	if diff := cmp.Diff([]int32{11}, w.killed); diff != "" {
		t.Errorf("killed (-want +got):\n%s", diff)
	}
	if want := "1 dev server running over a day"; !slices.Contains(m.lines(), want) {
		t.Errorf("menu lacks %q:\n%s", want, strings.Join(m.lines(), "\n"))
	}
	m.clickOn(t, "Stop It")
	if diff := cmp.Diff([]int32{11, 10}, w.killed); diff != "" {
		t.Errorf("killed (-want +got):\n%s", diff)
	}
	if slices.ContainsFunc(m.lines(), func(l string) bool { return strings.Contains(l, "running over") }) {
		t.Errorf("stale line still shown:\n%s", strings.Join(m.lines(), "\n"))
	}
}

func TestForgottenServersStopAllAndThreshold(t *testing.T) {
	t.Parallel()
	now := time.Now()
	a := inspect.Socket{RawSocket: listen(5173, 20), Process: &inspect.ProcessInfo{PID: 20, Name: "vite", User: "alice", Project: "a", CreateTime: now.Add(-80 * time.Hour)}}
	b := inspect.Socket{RawSocket: listen(8000, 21), Process: &inspect.ProcessInfo{PID: 21, Name: "python3", User: "alice", Project: "b", CreateTime: now.Add(-50 * time.Hour)}}
	w := &world{sockets: []inspect.Socket{a, b}}
	d := w.deps() // Now unset: the real clock
	d.StaleAfter = 48 * time.Hour
	m := &fakeMenu{}
	tr := New(d, m)
	tr.Refresh(context.Background())
	if want := "2 dev servers running over 2 days"; !slices.Contains(m.lines(), want) {
		t.Fatalf("menu lacks %q:\n%s", want, strings.Join(m.lines(), "\n"))
	}
	m.clickOn(t, "Stop All 2")
	if diff := cmp.Diff([]int32{20, 21}, w.killed); diff != "" {
		t.Errorf("killed oldest first (-want +got):\n%s", diff)
	}
	if e := (Entry{PID: 1, Project: "x"}); e.Stale(now, time.Hour) {
		t.Error("an entry without a start time can't be stale")
	}
}
