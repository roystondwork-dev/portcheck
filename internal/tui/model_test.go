package tui

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/roystondwork-dev/portcheck/internal/testutil"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
)

// ---- fixtures --------------------------------------------------------------

func ap(s string) netip.AddrPort { return netip.MustParseAddrPort(s) }

var (
	node  = &inspect.ProcessInfo{PID: 100, Name: "node", User: "alice"}
	pg    = &inspect.ProcessInfo{PID: 200, Name: "postgres", User: "postgres"}
	dns   = &inspect.ProcessInfo{PID: 300, Name: "mDNSResponder", User: "_mdns"}
	redis = &inspect.ProcessInfo{PID: 400, Name: "redis-server"} // no user

	sNode     = inspect.Socket{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("127.0.0.1:3000"), State: inspect.StateListen, PID: 100}, Process: node}
	sNodeConn = inspect.Socket{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("127.0.0.1:3000"), Remote: ap("127.0.0.1:51000"), State: inspect.StateEstablished, PID: 100}, Process: node}
	sPG       = inspect.Socket{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("[::]:5432"), State: inspect.StateListen, PID: 200}, Process: pg}
	sDNS      = inspect.Socket{RawSocket: inspect.RawSocket{Proto: inspect.UDP, Local: ap("0.0.0.0:5353"), State: inspect.StateNone, PID: 300}, Process: dns}
	sRedis    = inspect.Socket{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("127.0.0.1:6379"), State: inspect.StateListen, PID: 400}, Process: redis}
	sHidden   = inspect.Socket{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("0.0.0.0:22"), State: inspect.StateListen}}

	fixture = []inspect.Socket{sRedis, sNodeConn, sPG, sHidden, sDNS, sNode}
)

type fakeLister struct {
	mu    sync.Mutex
	res   *inspect.Result
	err   error
	calls int
}

func (l *fakeLister) List(context.Context, inspect.Filter) (*inspect.Result, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	return l.res, l.err
}

type fakeKiller struct {
	opts killer.Options
	pid  int32
	err  error
}

func (k *fakeKiller) Kill(_ context.Context, pid int32, opts killer.Options) (killer.Result, error) {
	k.pid, k.opts = pid, opts
	if k.err != nil {
		return killer.Result{}, k.err
	}
	return killer.Result{PID: pid, Outcome: killer.Terminated}, nil
}

// ---- helpers ---------------------------------------------------------------

func newModel(t *testing.T, socks []inspect.Socket, partial bool) (Model, *fakeLister, *fakeKiller) {
	t.Helper()
	l := &fakeLister{res: &inspect.Result{Sockets: socks, Partial: partial}}
	k := &fakeKiller{}
	m := New(t.Context(), Deps{Lister: l, Killer: k, Hint: "Re-run with sudo.", Interval: time.Millisecond})
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 14})
	m = step(t, m, m.refreshCmd()())
	return m, l, k
}

func step(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	nm, _ := m.Update(msg)
	return nm.(Model)
}

func stepCmd(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	nm, cmd := m.Update(msg)
	return nm.(Model), cmd
}

var namedKeys = map[string]tea.KeyPressMsg{
	"up": {Code: tea.KeyUp}, "down": {Code: tea.KeyDown}, "pgup": {Code: tea.KeyPgUp}, "pgdown": {Code: tea.KeyPgDown},
	"home": {Code: tea.KeyHome}, "end": {Code: tea.KeyEnd}, "tab": {Code: tea.KeyTab}, "esc": {Code: tea.KeyEscape},
	"enter": {Code: tea.KeyEnter}, "delete": {Code: tea.KeyDelete}, "ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
}

func key(k string) tea.KeyPressMsg {
	if msg, ok := namedKeys[k]; ok {
		return msg
	}
	r, _ := utf8.DecodeRuneInString(k)
	return tea.KeyPressMsg{Code: r, Text: k}
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		m = step(t, m, key(k))
	}
	return m
}

func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m = step(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func screen(m Model) string { return ansi.Strip(m.View().Content) }

func ports(m Model) []uint16 {
	out := make([]uint16, len(m.rows))
	for i, s := range m.rows {
		out[i] = s.Port()
	}
	return out
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// ---- tests -----------------------------------------------------------------

func TestInitRefreshesAndTicks(t *testing.T) {
	t.Parallel()
	l := &fakeLister{res: &inspect.Result{}}
	m := New(t.Context(), Deps{Lister: l, Interval: time.Millisecond})
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("Init() = %T, want a batch of 2", batch)
	}
	var gotRefresh, gotTick bool
	for _, c := range batch {
		switch c().(type) {
		case refreshMsg:
			gotRefresh = true
		case tickMsg:
			gotTick = true
		}
	}
	if !gotRefresh || !gotTick || l.calls != 1 {
		t.Errorf("refresh=%v tick=%v calls=%d", gotRefresh, gotTick, l.calls)
	}
}

func TestDefaults(t *testing.T) {
	t.Parallel()
	m := New(t.Context(), Deps{})
	if m.deps.Interval != 2*time.Second || m.deps.Grace != 3*time.Second || !m.listeningOnly || !m.loading {
		t.Errorf("defaults: %+v", m.deps)
	}
	v := m.View()
	if !v.AltScreen || v.WindowTitle != "portcheck" {
		t.Error("view must use the alt screen and set a title")
	}
	if !strings.Contains(screen(m), "loading…") {
		t.Errorf("initial screen:\n%s", screen(m))
	}
}

func TestGoldenScreens(t *testing.T) {
	t.Parallel()
	// Each subtest builds its own model: textinput.Model copies share
	// internal pointers, so models must not be shared across goroutines.
	cases := map[string]func(Model) Model{
		"loaded":    func(m Model) Model { return m },
		"all":       func(m Model) Model { return press(t, m, "l") },
		"filtering": func(m Model) Model { return typeText(t, press(t, m, "/"), "post") },
		"filtered":  func(m Model) Model { return press(t, typeText(t, press(t, m, "/"), "post"), "enter") },
		"confirm":   func(m Model) Model { return press(t, m, "down", "x") },
		"force":     func(m Model) Model { return press(t, m, "end", "K") },
		"help":      func(m Model) Model { return press(t, m, "?") },
		"narrow":    func(m Model) Model { return step(t, m, tea.WindowSizeMsg{Width: 40, Height: 8}) },
		"scrolled": func(m Model) Model {
			m = step(t, press(t, m, "l"), tea.WindowSizeMsg{Width: 100, Height: 7})
			return press(t, m, "end")
		},
		"empty":       func(m Model) Model { return typeText(t, press(t, m, "/"), "zzz") },
		"error":       func(m Model) Model { return step(t, m, refreshMsg{err: errors.New("lsof not found")}) },
		"desc-by-pid": func(m Model) Model { return press(t, m, "s", "s", "s", "S") },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base, _, _ := newModel(t, fixture, true)
			testutil.Golden(t, "screen-"+name, []byte(screen(fn(base))+"\n"))
		})
	}
}

func TestErrorWithoutData(t *testing.T) {
	t.Parallel()
	m := New(t.Context(), Deps{})
	m = step(t, m, refreshMsg{err: errors.New("boom")})
	if !strings.Contains(screen(m), "error: boom") {
		t.Errorf("screen:\n%s", screen(m))
	}
	m = step(t, m, refreshMsg{res: &inspect.Result{Sockets: []inspect.Socket{sNode}}})
	if m.err != nil || strings.Contains(screen(m), "boom") {
		t.Error("successful refresh must clear the error")
	}
}

func TestNavigation(t *testing.T) {
	t.Parallel()
	m, _, _ := newModel(t, fixture, false)
	m = press(t, m, "l") // 6 rows
	if len(m.rows) != 6 {
		t.Fatalf("rows = %d", len(m.rows))
	}
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 7}) // page size 3
	steps := []struct {
		key          string
		cursor, offs int
	}{
		{"down", 1, 0}, {"j", 2, 0}, {"down", 3, 1}, {"pgdown", 5, 3}, {"down", 5, 3},
		{"pgup", 2, 2}, {"up", 1, 1}, {"home", 0, 0}, {"up", 0, 0}, {"G", 5, 3}, {"g", 0, 0}, {"end", 5, 3},
	}
	for _, s := range steps {
		m = press(t, m, s.key)
		if m.cursor != s.cursor || m.offset != s.offs {
			t.Fatalf("after %q: cursor=%d offset=%d, want %d/%d", s.key, m.cursor, m.offset, s.cursor, s.offs)
		}
	}
	// Growing the window pulls the offset back so no blank page is shown.
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	if m.offset != 0 || m.cursor != 5 {
		t.Errorf("after resize: cursor=%d offset=%d", m.cursor, m.offset)
	}
	// Tiny terminal still shows one row.
	if m = step(t, m, tea.WindowSizeMsg{Width: 10, Height: 2}); m.pageSize() != 1 {
		t.Errorf("pageSize = %d", m.pageSize())
	}
}

func TestProtoAndListeningToggles(t *testing.T) {
	t.Parallel()
	m, _, _ := newModel(t, fixture, false)
	if got := ports(m); !equal(got, 22, 3000, 5353, 5432, 6379) {
		t.Errorf("listening = %v", got)
	}
	m = press(t, m, "tab")
	if got := ports(m); !equal(got, 22, 3000, 5432, 6379) || !strings.Contains(screen(m), "· tcp ·") {
		t.Errorf("tcp = %v", got)
	}
	m = press(t, m, "tab")
	if got := ports(m); !equal(got, 5353) || !strings.Contains(screen(m), "· udp ·") {
		t.Errorf("udp = %v", got)
	}
	m = press(t, m, "tab", "l")
	if got := ports(m); !equal(got, 22, 3000, 3000, 5353, 5432, 6379) || !strings.Contains(screen(m), "all sockets") {
		t.Errorf("all = %v", got)
	}
}

func TestSorting(t *testing.T) {
	t.Parallel()
	m, _, _ := newModel(t, fixture, false)
	want := map[column][]uint16{
		colProto:   {22, 3000, 5432, 6379, 5353},
		colState:   {22, 3000, 5432, 6379, 5353},
		colPID:     {22, 3000, 5432, 5353, 6379},
		colProcess: {22, 5353, 3000, 5432, 6379},
		colUser:    {22, 6379, 5353, 3000, 5432},
		colPort:    {22, 3000, 5353, 5432, 6379},
	}
	for range numSortCols {
		m = press(t, m, "s")
		if got := ports(m); !equal(got, want[m.sortCol]...) {
			t.Errorf("sort by %s = %v, want %v", m.sortCol, got, want[m.sortCol])
		}
	}
	m = press(t, m, "S")
	if got := ports(m); !equal(got, 6379, 5432, 5353, 3000, 22) || !strings.Contains(screen(m), "sort: port ↓") {
		t.Errorf("descending = %v", got)
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()
	m, _, _ := newModel(t, fixture, false)
	m = typeText(t, press(t, m, "/"), "ALICE")
	if m.mode != modeFilter || !equal(ports(m), 3000) {
		t.Fatalf("live filter: mode=%v rows=%v", m.mode, ports(m))
	}
	m = typeText(t, m, "q") // 'q' is text while filtering, not quit
	if m.filter.Value() != "ALICEq" {
		t.Errorf("filter value = %q", m.filter.Value())
	}
	m = press(t, m, "esc")
	if m.mode != modeNormal || m.filter.Value() != "" || len(m.rows) != 5 {
		t.Errorf("esc in filter mode must clear: mode=%v value=%q rows=%d", m.mode, m.filter.Value(), len(m.rows))
	}
	// Search covers remote addresses, pids and states too.
	m = press(t, typeText(t, press(t, m, "l", "/"), "51000"), "enter")
	if !equal(ports(m), 3000) || m.rows[0].State != inspect.StateEstablished {
		t.Errorf("remote search = %v", ports(m))
	}
	m = press(t, m, "esc")
	if m.filter.Value() != "" || len(m.rows) != 6 {
		t.Error("esc in normal mode must clear the filter")
	}
}

func TestKillFlow(t *testing.T) {
	t.Parallel()
	m, l, k := newModel(t, fixture, false)
	m = press(t, m, "down") // node on 3000
	m = press(t, m, "x")
	if m.mode != modeConfirm || m.confirm.pid != 100 || m.confirm.force {
		t.Fatalf("confirm = %+v mode %v", m.confirm, m.mode)
	}
	m, cmd := stepCmd(t, m, key("y"))
	if m.mode != modeNormal || cmd == nil || !strings.Contains(m.status, "stopping node (pid 100)") {
		t.Fatalf("after y: mode=%v status=%q", m.mode, m.status)
	}
	done := cmd()
	if k.pid != 100 || k.opts != (killer.Options{Grace: 3 * time.Second}) {
		t.Errorf("Kill(%d, %+v)", k.pid, k.opts)
	}
	callsBefore := l.calls
	m, cmd = stepCmd(t, m, done)
	if !strings.Contains(screen(m), "terminated node (pid 100)") || m.statusTone != toneGood || !m.loading {
		t.Errorf("status = %q", m.status)
	}
	if _, ok := cmd().(refreshMsg); !ok || l.calls != callsBefore+1 {
		t.Error("kill completion must trigger a refresh")
	}
}

func TestForceKillAndErrors(t *testing.T) {
	t.Parallel()
	m, _, k := newModel(t, fixture, false)
	k.err = killer.ErrPermission
	m = press(t, m, "end", "K")
	if !strings.Contains(screen(m), "Force-kill redis-server (pid 400) listening on 6379? [y/N]") {
		t.Fatalf("screen:\n%s", screen(m))
	}
	m, cmd := stepCmd(t, m, key("Y"))
	if !strings.Contains(m.status, "force-killing") {
		t.Errorf("status = %q", m.status)
	}
	m = step(t, m, cmd())
	if !k.opts.Force || m.statusTone != toneBad || !strings.Contains(screen(m), "could not stop redis-server (pid 400): permission denied") {
		t.Errorf("opts=%+v status=%q", k.opts, m.status)
	}
}

func TestKillCancelAndEdgeCases(t *testing.T) {
	t.Parallel()
	m, _, k := newModel(t, fixture, false)
	m = press(t, m, "down", "delete", "n")
	if m.mode != modeNormal || !strings.Contains(screen(m), "cancelled") || k.pid != 0 {
		t.Errorf("cancel: mode=%v status=%q", m.mode, m.status)
	}
	m = press(t, m, "home", "k") // hidden owner on 22
	if m.mode != modeNormal || !strings.Contains(m.status, "owner of port 22 is not visible. Re-run with sudo.") {
		t.Errorf("hidden owner: %q", m.status)
	}
	// Unknown process name still confirms with "?".
	m, _, _ = newModel(t, []inspect.Socket{{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("0.0.0.0:9"), State: inspect.StateListen, PID: 9}}}, false)
	if m = press(t, m, "x"); m.confirm.name != "?" {
		t.Errorf("name = %q", m.confirm.name)
	}
	// Nothing selected: no-op.
	m, _, _ = newModel(t, nil, false)
	if m = press(t, m, "x"); m.mode != modeNormal {
		t.Error("kill with no rows changed mode")
	}
}

func TestCopyPID(t *testing.T) {
	t.Parallel()
	m, _, _ := newModel(t, fixture, false)
	m, cmd := stepCmd(t, m, key("c")) // hidden owner: nothing to copy
	if cmd != nil || m.status != "" {
		t.Error("copy on hidden owner should do nothing")
	}
	m = press(t, m, "down")
	m, cmd = stepCmd(t, m, key("c"))
	if cmd == nil || m.status != "copied pid 100 to clipboard" {
		t.Errorf("copy: cmd=%v status=%q", cmd != nil, m.status)
	}
}

func TestRefreshAndTick(t *testing.T) {
	t.Parallel()
	m, l, _ := newModel(t, fixture, false)
	m, cmd := stepCmd(t, m, key("r"))
	if !m.loading || cmd == nil || !strings.Contains(screen(m), "refreshing") {
		t.Fatal("r must start a refresh")
	}
	if _, cmd2 := stepCmd(t, m, key("r")); cmd2 != nil {
		t.Error("r while loading must not start another refresh")
	}
	if _, cmd2 := stepCmd(t, m, tickMsg{}); cmd2 == nil {
		t.Error("tick while loading must reschedule")
	} else if _, ok := cmd2().(tickMsg); !ok {
		t.Error("tick while loading must only reschedule the tick")
	}
	m = step(t, m, cmd())
	m, cmd = stepCmd(t, m, tickMsg{})
	if !m.loading {
		t.Fatal("tick when idle must refresh")
	}
	if batch, ok := cmd().(tea.BatchMsg); !ok || len(batch) != 2 {
		t.Error("tick when idle must refresh and reschedule")
	}
	_ = l
}

func TestCursorFollowsSocketAcrossRefresh(t *testing.T) {
	t.Parallel()
	m, _, _ := newModel(t, fixture, false)
	m = press(t, m, "down", "down", "down") // 5432 postgres
	extra := inspect.Socket{RawSocket: inspect.RawSocket{Proto: inspect.TCP, Local: ap("0.0.0.0:1"), State: inspect.StateListen, PID: 1}}
	m = step(t, m, refreshMsg{res: &inspect.Result{Sockets: append([]inspect.Socket{extra}, fixture...)}})
	if s, _ := m.selected(); s.Port() != 5432 {
		t.Errorf("selection moved to %d", s.Port())
	}
	// If the selected socket disappears the cursor is clamped.
	m = step(t, m, refreshMsg{res: &inspect.Result{Sockets: []inspect.Socket{sNode}}})
	if m.cursor != 0 {
		t.Errorf("cursor = %d", m.cursor)
	}
}

func TestStatusLinePriorities(t *testing.T) {
	t.Parallel()
	m, _, _ := newModel(t, fixture, true)
	if !strings.Contains(screen(m), "some owners are hidden. Re-run with sudo.") {
		t.Error("partial hint missing")
	}
	m.deps.Hint = ""
	if strings.Contains(screen(m), "some owners are hidden") {
		t.Error("hint shown although empty")
	}
	m = step(t, m, refreshMsg{err: errors.New("flaky")})
	if !strings.Contains(screen(m), "refresh failed: flaky") {
		t.Error("refresh error not shown while keeping old rows")
	}
	m.setStatus(toneInfo, "plain info")
	if !strings.Contains(screen(m), "plain info") {
		t.Error("info status not shown")
	}
}

func TestHelpAndQuit(t *testing.T) {
	t.Parallel()
	m, _, _ := newModel(t, fixture, false)
	m = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = press(t, m, "?")
	if m.mode != modeHelp || !strings.Contains(screen(m), "press any key to return") {
		t.Fatal("help not shown")
	}
	m, cmd := stepCmd(t, m, key("q")) // any key closes help, does not quit
	if m.mode != modeNormal || isQuit(cmd) {
		t.Error("key in help mode should only close help")
	}
	if _, cmd := stepCmd(t, m, key("q")); !isQuit(cmd) {
		t.Error("q must quit")
	}
	for _, mode := range []mode{modeNormal, modeFilter, modeConfirm, modeHelp} {
		m.mode = mode
		if _, cmd := stepCmd(t, m, key("ctrl+c")); !isQuit(cmd) {
			t.Errorf("ctrl+c must quit in mode %d", mode)
		}
	}
}

func TestIgnoresUnknownMessagesAndKeys(t *testing.T) {
	t.Parallel()
	m, _, _ := newModel(t, fixture, false)
	before := screen(m)
	m, cmd := stepCmd(t, m, struct{}{})
	m2, cmd2 := stepCmd(t, m, key("z"))
	if cmd != nil || cmd2 != nil || screen(m2) != before {
		t.Error("unknown input changed state")
	}
}

func TestStrings(t *testing.T) {
	t.Parallel()
	if protoAll.String() != "tcp+udp" || protoTCP.String() != "tcp" || protoUDP.String() != "udp" {
		t.Error("protoFilter strings")
	}
	if colUser.String() != "user" || colPort.String() != "port" {
		t.Error("column strings")
	}
}

func TestRunQuitsOnQ(t *testing.T) {
	t.Parallel()
	l := &fakeLister{res: &inspect.Result{Sockets: fixture}}
	var out bytes.Buffer
	err := Run(t.Context(), Deps{Lister: l, Interval: time.Hour}, strings.NewReader("q"), &out, []string{"TERM=xterm-256color"})
	if err != nil {
		t.Fatalf("Run = %v", err)
	}
	if out.Len() == 0 {
		t.Error("nothing rendered")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("tty gone") }

func TestRunInputError(t *testing.T) {
	t.Parallel()
	err := Run(t.Context(), Deps{Lister: &fakeLister{res: &inspect.Result{}}, Interval: time.Hour}, errReader{}, &bytes.Buffer{}, nil)
	if err == nil || errors.Is(err, context.Canceled) {
		t.Errorf("Run = %v, want a non-cancellation error", err)
	}
}

func TestRowStyles(t *testing.T) {
	t.Parallel()
	m, _, _ := newModel(t, fixture, false)
	m = press(t, m, "down") // cursor on 3000; 22 has a hidden owner
	content := m.View().Content
	if !strings.Contains(content, "\x1b[90m22 ") {
		t.Errorf("hidden-owner row is not dimmed:\n%q", content)
	}
	if !strings.Contains(content, "\x1b[32m5432 ") {
		t.Errorf("listening row is not green:\n%q", content)
	}
	if !strings.Contains(content, "\x1b[7m3000 ") {
		t.Errorf("selected row is not reversed:\n%q", content)
	}
}

func TestRunCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := Run(ctx, Deps{Lister: &fakeLister{res: &inspect.Result{}}}, strings.NewReader(""), &bytes.Buffer{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run = %v, want context.Canceled", err)
	}
}

func equal(got []uint16, want ...uint16) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
