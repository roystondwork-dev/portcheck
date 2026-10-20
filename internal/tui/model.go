// Package tui implements portcheck's interactive terminal UI: a live,
// sortable, filterable table of sockets from which processes can be stopped.
//
// The Model is pure: Update never performs I/O itself. Listing sockets and
// killing processes run as tea.Cmds through the Lister and Killer interfaces,
// which makes every interaction unit-testable.
package tui

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
)

// Lister lists sockets; implemented by *inspect.Inspector.
type Lister interface {
	List(ctx context.Context, f inspect.Filter) (*inspect.Result, error)
}

// Killer stops processes; implemented by *killer.Killer.
type Killer interface {
	Kill(ctx context.Context, pid int32, opts killer.Options) (killer.Result, error)
}

// Deps are the TUI's dependencies.
type Deps struct {
	Lister Lister
	Killer Killer
	// Interval is the refresh period (default 2s).
	Interval time.Duration
	// Hint is shown when some socket owners are hidden (see inspect.PrivilegeHint).
	Hint string
	// Grace is passed to killer.Options for graceful kills (default 3s).
	Grace time.Duration
}

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeConfirm
	modeHelp
)

type protoFilter int

const (
	protoAll protoFilter = iota
	protoTCP
	protoUDP
)

func (p protoFilter) String() string {
	switch p {
	case protoTCP:
		return "tcp"
	case protoUDP:
		return "udp"
	default:
		return "tcp+udp"
	}
}

// column identifies a sortable column.
type column int

const (
	colPort column = iota
	colProto
	colState
	colPID
	colProcess
	colUser
	numSortCols
)

func (c column) String() string {
	return [...]string{"port", "proto", "state", "pid", "process", "user"}[c]
}

type tone int

const (
	toneInfo tone = iota
	toneGood
	toneBad
)

// Messages.
type (
	refreshMsg struct {
		res *inspect.Result
		err error
	}
	tickMsg     struct{}
	killDoneMsg struct {
		target killTarget
		res    killer.Result
		err    error
	}
)

type killTarget struct {
	pid      int32
	name     string
	port     uint16
	force    bool
	ancestry string
}

// Model is the Bubble Tea model.
type Model struct {
	ctx  context.Context
	deps Deps

	width, height int

	all     []inspect.Socket // last refresh
	rows    []inspect.Socket // filtered and sorted view of all
	partial bool
	err     error
	loading bool

	cursor, offset int
	sortCol        column
	sortDesc       bool
	proto          protoFilter
	listeningOnly  bool

	mode    mode
	filter  textinput.Model
	confirm killTarget

	status     string
	statusTone tone
}

// New returns a Model listening-only, sorted by port, 80×24 until the first
// WindowSizeMsg.
func New(ctx context.Context, deps Deps) Model {
	if deps.Interval <= 0 {
		deps.Interval = 2 * time.Second
	}
	if deps.Grace <= 0 {
		deps.Grace = 3 * time.Second
	}
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "filter by port, process, user, address…"
	return Model{ctx: ctx, deps: deps, width: 80, height: 24, listeningOnly: true, filter: ti, loading: true}
}

// Init starts the first refresh and the refresh timer.
func (m Model) Init() tea.Cmd { return tea.Batch(m.refreshCmd(), m.tickCmd()) }

func (m Model) refreshCmd() tea.Cmd {
	lister, ctx := m.deps.Lister, m.ctx
	return func() tea.Msg {
		res, err := lister.List(ctx, inspect.Filter{})
		return refreshMsg{res: res, err: err}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.deps.Interval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) killCmd(t killTarget) tea.Cmd {
	k, ctx, grace := m.deps.Killer, m.ctx, m.deps.Grace
	return func() tea.Msg {
		res, err := k.Kill(ctx, t.pid, killer.Options{Grace: grace, Force: t.force})
		return killDoneMsg{target: t, res: res, err: err}
	}
}

// Update handles a message.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampCursor()
		return m, nil
	case tickMsg:
		if m.loading {
			return m, m.tickCmd()
		}
		m.loading = true
		return m, tea.Batch(m.refreshCmd(), m.tickCmd())
	case refreshMsg:
		return m.onRefresh(msg), nil
	case killDoneMsg:
		m.onKillDone(msg)
		m.loading = true
		return m, m.refreshCmd()
	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m Model) onRefresh(msg refreshMsg) Model {
	m.loading = false
	if msg.err != nil {
		m.err = msg.err
		return m
	}
	m.err = nil
	selected, hadSelection := m.selected()
	m.all, m.partial = msg.res.Sockets, msg.res.Partial
	m.rebuild()
	if hadSelection {
		// Keep the cursor on the same socket across refreshes.
		if i := slices.IndexFunc(m.rows, func(s inspect.Socket) bool { return s.RawSocket == selected.RawSocket }); i >= 0 {
			m.cursor = i
		}
	}
	m.clampCursor()
	return m
}

func (m *Model) onKillDone(msg killDoneMsg) {
	t := msg.target
	if msg.err != nil {
		m.setStatus(toneBad, "could not stop %s (pid %d): %v", t.name, t.pid, msg.err)
		return
	}
	m.setStatus(toneGood, "%s %s (pid %d)", msg.res.Outcome, t.name, t.pid)
}

func (m *Model) setStatus(t tone, format string, args ...any) {
	m.status, m.statusTone = fmt.Sprintf(format, args...), t
}

func (m Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.mode {
	case modeHelp:
		m.mode = modeNormal
		return m, nil
	case modeConfirm:
		return m.onConfirmKey(key)
	case modeFilter:
		return m.onFilterKey(msg)
	default:
		return m.onNormalKey(key)
	}
}

func (m Model) onConfirmKey(key string) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	if key == "y" || key == "Y" {
		verb := "stopping"
		if m.confirm.force {
			verb = "force-killing"
		}
		m.setStatus(toneInfo, "%s %s (pid %d)…", verb, m.confirm.name, m.confirm.pid)
		return m, m.killCmd(m.confirm)
	}
	m.setStatus(toneInfo, "cancelled")
	return m, nil
}

func (m Model) onFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.mode = modeNormal
		m.filter.Blur()
		return m, nil
	case "esc":
		m.mode = modeNormal
		m.filter.Blur()
		m.filter.SetValue("")
		m.rebuild()
		return m, nil
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	m.rebuild()
	return m, cmd
}

func (m Model) onNormalKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, tea.Quit
	case "up":
		m.cursor--
	case "down", "j":
		m.cursor++
	case "pgup":
		m.cursor -= m.pageSize()
	case "pgdown":
		m.cursor += m.pageSize()
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.rows) - 1
	case "/":
		m.mode = modeFilter
		return m, m.filter.Focus()
	case "esc":
		m.filter.SetValue("")
		m.rebuild()
	case "tab":
		m.proto = (m.proto + 1) % 3
		m.rebuild()
	case "l":
		m.listeningOnly = !m.listeningOnly
		m.rebuild()
	case "s":
		m.sortCol = (m.sortCol + 1) % numSortCols
		m.rebuild()
	case "S":
		m.sortDesc = !m.sortDesc
		m.rebuild()
	case "r":
		if !m.loading {
			m.loading = true
			return m, m.refreshCmd()
		}
	case "?":
		m.mode = modeHelp
	case "c":
		if s, ok := m.selected(); ok && s.PID > 0 {
			m.setStatus(toneInfo, "copied pid %d to clipboard", s.PID)
			return m, tea.SetClipboard(strconv.Itoa(int(s.PID)))
		}
	case "x", "delete", "k", "K":
		return m.askKill(key == "K")
	}
	m.clampCursor()
	return m, nil
}

func (m Model) askKill(force bool) (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	switch {
	case !ok:
		return m, nil
	case s.Container != nil:
		ref := cmp.Or(s.Container.Name, s.Container.ID)
		m.setStatus(toneBad, "port %d is published by %s container %q; stop it with: %s stop %s",
			s.Port(), s.Container.Runtime, ref, s.Container.Runtime, ref)
		return m, nil
	case s.PID <= 0:
		m.setStatus(toneBad, "owner of port %d is not visible. %s", s.Port(), m.deps.Hint)
		return m, nil
	}
	name := "?"
	if s.Process != nil {
		name = s.Process.Name
	}
	m.confirm = killTarget{pid: s.PID, name: name, port: s.Port(), force: force, ancestry: output.Ancestry(s.Process)}
	m.mode = modeConfirm
	return m, nil
}

func (m Model) selected() (inspect.Socket, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return inspect.Socket{}, false
	}
	return m.rows[m.cursor], true
}

// rebuild recomputes rows from all according to filters and sort order.
func (m *Model) rebuild() {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	m.rows = m.rows[:0:0]
	for _, s := range m.all {
		if m.listeningOnly && !s.IsListening() {
			continue
		}
		if (m.proto == protoTCP && s.Proto != inspect.TCP) || (m.proto == protoUDP && s.Proto != inspect.UDP) {
			continue
		}
		if q != "" && !strings.Contains(searchText(s), q) {
			continue
		}
		m.rows = append(m.rows, s)
	}
	slices.SortStableFunc(m.rows, func(a, b inspect.Socket) int {
		c := compareBy(m.sortCol, a, b)
		if m.sortDesc {
			c = -c
		}
		return cmp.Or(c, cmp.Compare(a.Port(), b.Port()))
	})
	m.clampCursor()
}

func searchText(s inspect.Socket) string {
	parts := []string{strconv.Itoa(int(s.Port())), string(s.Proto), s.Local.String(), string(s.State), strconv.Itoa(int(s.PID))}
	if s.Remote.IsValid() {
		parts = append(parts, s.Remote.String())
	}
	if p := s.Process; p != nil {
		parts = append(parts, p.Name, p.User)
	}
	if c := s.Container; c != nil {
		parts = append(parts, containerLabel(c), c.Image)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// containerLabel names a container for display, e.g. "docker:web".
func containerLabel(c *inspect.Container) string {
	return c.Runtime + ":" + cmp.Or(c.Name, c.Image, c.ID)
}

func processName(s inspect.Socket) string {
	switch {
	case s.Container != nil:
		return containerLabel(s.Container)
	case s.Process != nil:
		return s.Process.Name
	default:
		return ""
	}
}

func processUser(s inspect.Socket) string {
	if s.Process == nil {
		return ""
	}
	return s.Process.User
}

func compareBy(c column, a, b inspect.Socket) int {
	switch c {
	case colProto:
		return cmp.Compare(a.Proto, b.Proto)
	case colState:
		return cmp.Compare(a.State, b.State)
	case colPID:
		return cmp.Compare(a.PID, b.PID)
	case colProcess:
		return cmp.Compare(strings.ToLower(processName(a)), strings.ToLower(processName(b)))
	case colUser:
		return cmp.Compare(processUser(a), processUser(b))
	default:
		return cmp.Compare(a.Port(), b.Port())
	}
}

// Layout: title, column header, rows…, status, footer.
const chromeLines = 4

func (m Model) pageSize() int { return max(m.height-chromeLines, 1) }

func (m *Model) clampCursor() {
	m.cursor = min(max(m.cursor, 0), max(len(m.rows)-1, 0))
	page := m.pageSize()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+page {
		m.offset = m.cursor - page + 1
	}
	m.offset = min(max(m.offset, 0), max(len(m.rows)-page, 0))
}
