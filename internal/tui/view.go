package tui

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	headerStyle   = lipgloss.NewStyle().Bold(true).Underline(true)
	selectedStyle = lipgloss.NewStyle().Reverse(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	listenStyle   = lipgloss.NewStyle().Foreground(lipgloss.Green)
	goodStyle     = lipgloss.NewStyle().Foreground(lipgloss.Green)
	badStyle      = lipgloss.NewStyle().Foreground(lipgloss.Red).Bold(true)
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	keyStyle      = lipgloss.NewStyle().Bold(true)
)

// columns in display order; widths are computed from the visible rows.
var headers = []string{"PORT", "PROTO", "LOCAL ADDRESS", "REMOTE ADDRESS", "STATE", "PID", "PROCESS", "USER"}

// maxWidths caps free-text columns so one long name can't push others off screen.
var maxWidths = []int{5, 5, 40, 40, 11, 7, 24, 16}

// View renders the UI.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "portcheck"
	return v
}

func (m Model) render() string {
	if m.mode == modeHelp {
		return m.fit(helpText())
	}
	lines := make([]string, 0, m.height)
	lines = append(lines, m.titleLine())

	cells := make([][]string, len(m.rows))
	for i, s := range m.rows {
		cells[i] = rowCells(s)
	}
	widths := columnWidths(cells)
	lines = append(lines, headerStyle.Render(joinCells(headers, widths)))

	page := m.pageSize()
	switch {
	case m.err != nil && len(m.rows) == 0:
		lines = append(lines, badStyle.Render("error: "+m.err.Error()))
	case m.loading && len(m.all) == 0:
		lines = append(lines, mutedStyle.Render("loading…"))
	case len(m.rows) == 0:
		lines = append(lines, mutedStyle.Render("no matching sockets — press l to include non-listening sockets, esc to clear the filter"))
	}
	for i := m.offset; i < len(m.rows) && i < m.offset+page; i++ {
		line := joinCells(cells[i], widths)
		switch {
		case i == m.cursor:
			line = selectedStyle.Render(line)
		case m.rows[i].PID <= 0: // owner unknown: dim, even when listening
			line = mutedStyle.Render(line)
		case m.rows[i].State == inspect.StateListen:
			line = listenStyle.Render(line)
		}
		lines = append(lines, line)
	}
	for len(lines) < m.height-2 {
		lines = append(lines, "")
	}
	lines = append(lines, m.statusLine(), m.footerLine())
	return m.fit(strings.Join(lines, "\n"))
}

// fit truncates every line to the terminal width and the text to its height.
func (m Model) fit(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.width, "…")
	}
	return strings.Join(lines, "\n")
}

func (m Model) titleLine() string {
	scope := "all sockets"
	if m.listeningOnly {
		scope = "listening"
	}
	order := "↑"
	if m.sortDesc {
		order = "↓"
	}
	title := titleStyle.Render("portcheck") + fmt.Sprintf("  %d %s · %s · sort: %s %s", len(m.rows), scope, m.proto, m.sortCol, order)
	if m.loading {
		title += mutedStyle.Render(" · refreshing")
	}
	if f := m.filter.Value(); f != "" && m.mode != modeFilter {
		title += " · filter: " + warnStyle.Render(f)
	}
	return title
}

func (m Model) statusLine() string {
	switch {
	case m.mode == modeFilter:
		return m.filter.View()
	case m.mode == modeConfirm:
		verb := "Stop"
		if m.confirm.force {
			verb = "Force-kill"
		}
		return warnStyle.Render(fmt.Sprintf("%s %s (pid %d) listening on %d? [y/N]", verb, m.confirm.name, m.confirm.pid, m.confirm.port))
	case m.status != "":
		switch m.statusTone {
		case toneGood:
			return goodStyle.Render(m.status)
		case toneBad:
			return badStyle.Render(m.status)
		default:
			return m.status
		}
	case m.err != nil:
		return badStyle.Render("refresh failed: " + m.err.Error())
	case m.partial && m.deps.Hint != "":
		return mutedStyle.Render("some owners are hidden. " + m.deps.Hint)
	default:
		return ""
	}
}

func (m Model) footerLine() string {
	keys := [][2]string{{"↑↓", "move"}, {"/", "filter"}, {"tab", "proto"}, {"l", "listening"}, {"s", "sort"}, {"x", "kill"}, {"?", "help"}, {"q", "quit"}}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = keyStyle.Render(k[0]) + " " + mutedStyle.Render(k[1])
	}
	return strings.Join(parts, mutedStyle.Render(" · "))
}

func helpText() string {
	rows := [][2]string{
		{"↑ / ↓ / j", "move selection"},
		{"pgup / pgdown", "move one page"},
		{"home g / end G", "jump to first / last"},
		{"/", "filter (enter to keep, esc to clear)"},
		{"esc", "clear filter"},
		{"tab", "cycle protocol: tcp+udp → tcp → udp"},
		{"l", "toggle listening-only / all sockets"},
		{"s / S", "next sort column / reverse order"},
		{"r", "refresh now"},
		{"c", "copy PID to clipboard"},
		{"x / k / delete", "stop the selected process (asks first)"},
		{"K", "force-kill the selected process (asks first)"},
		{"?", "toggle this help"},
		{"q / ctrl+c", "quit"},
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("portcheck — keys") + "\n\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "  %s  %s\n", keyStyle.Render(fmt.Sprintf("%-16s", r[0])), r[1])
	}
	b.WriteString("\n" + mutedStyle.Render("press any key to return"))
	return b.String()
}

func rowCells(s inspect.Socket) []string {
	remote := "-"
	if s.Remote.IsValid() {
		remote = s.Remote.String()
	}
	pid := "-"
	if s.PID > 0 {
		pid = strconv.Itoa(int(s.PID))
	}
	name, user := "?", "?"
	if s.Process != nil {
		name, user = s.Process.Name, cmp.Or(s.Process.User, "-")
	}
	if s.Container != nil {
		name = containerLabel(s.Container)
	}
	return []string{strconv.Itoa(int(s.Port())), string(s.Proto), s.Local.String(), remote, string(s.State), pid, name, user}
}

func columnWidths(rows [][]string) []int {
	w := make([]int, len(headers))
	for i, h := range headers {
		w[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range r {
			w[i] = max(w[i], min(ansi.StringWidth(c), maxWidths[i]))
		}
	}
	return w
}

func joinCells(cells []string, widths []int) string {
	parts := make([]string, len(cells))
	for i, c := range cells {
		c = ansi.Truncate(c, widths[i], "…")
		if i < len(cells)-1 {
			c += strings.Repeat(" ", widths[i]-ansi.StringWidth(c))
		}
		parts[i] = c
	}
	return strings.Join(parts, "  ")
}
