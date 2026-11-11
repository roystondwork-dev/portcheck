// Package tray is the logic of portcheck-tray, the menu bar (macOS) and
// system tray (Windows, Linux) app: which ports to show, and what each menu
// item does. The OS tray library sits behind Menu, so all of it is testable.
package tray

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
)

// Entry is one listening port in the menu.
type Entry struct {
	Port      uint16
	Name      string // e.g. "node", "docker:web"
	PID       int32  // 0 for containers and hidden owners
	StartedBy string // "npm (4200) ← zsh (900)", or ""
	Cmdline   string
	StopHint  string    // for containers: the command that stops them
	Hidden    bool      // owner not visible to this user
	Project   string    // the project the process runs in, e.g. "my-shop"
	Started   time.Time // when the process started; zero if unknown
}

// URL opens the port in a browser.
func (e Entry) URL() string { return fmt.Sprintf("http://localhost:%d/", e.Port) }

// Label is the menu item's title, e.g. ":3000  node" or ":3000  node · my-shop".
func (e Entry) Label() string {
	if e.Project != "" {
		return fmt.Sprintf(":%d  %s · %s", e.Port, e.Name, e.Project)
	}
	return fmt.Sprintf(":%d  %s", e.Port, e.Name)
}

// DefaultStaleAfter is how long a project's server runs before the tray
// suggests it may be forgotten.
const DefaultStaleAfter = 24 * time.Hour

// Stale reports whether e is one of the user's dev servers (a process in a
// project) that has been running at least after.
func (e Entry) Stale(now time.Time, after time.Duration) bool {
	return e.PID != 0 && e.Project != "" && !e.Started.IsZero() && now.Sub(e.Started) >= after
}

// Entries are the listening TCP ports to show, one per port and process,
// sorted by port. Unless all is set, only processes of user and containers:
// the ports a developer started, not system services.
func Entries(sockets []inspect.Socket, user string, all bool) []Entry {
	seen := map[string]bool{}
	var out []Entry
	for _, s := range sockets {
		if s.Proto != inspect.TCP || !s.IsListening() {
			continue
		}
		e := Entry{Port: s.Port()}
		switch {
		case s.Container != nil:
			e.Name = s.Container.Runtime + ":" + cmp.Or(s.Container.Name, s.Container.Image, s.Container.ID, "?")
			e.StopHint = output.StopHint(s.Container)
		case s.PID == 0:
			e.Name, e.Hidden = "(hidden)", true
		default:
			e.PID, e.Name = s.PID, "?"
			if p := s.Process; p != nil {
				e.Name, e.Cmdline, e.Project, e.Started = p.Name, p.Cmdline, p.Project, p.CreateTime
				var by []string
				for _, a := range p.Ancestors {
					by = append(by, fmt.Sprintf("%s (%d)", a.Name, a.PID))
				}
				e.StartedBy = strings.Join(by, " ← ")
			}
		}
		mine := s.Container != nil || (s.Process != nil && s.Process.User == user)
		if !all && !mine {
			continue
		}
		key := fmt.Sprintf("%d/%d/%s", e.Port, e.PID, e.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b Entry) int { return cmp.Or(cmp.Compare(a.Port, b.Port), strings.Compare(a.Name, b.Name)) })
	return out
}

// Menu is the tray's menu, as the OS tray library provides it.
type Menu interface {
	Reset()
	Add(title, tooltip string) Item
	AddCheckbox(title string, checked bool) Item
	Separator()
	SetTitle(title, tooltip string)
	Quit()
}

// Item is a menu item.
type Item interface {
	Sub(title, tooltip string) Item
	Disable()
	OnClick(func())
}

// Deps is what the tray needs from the outside world.
type Deps struct {
	List  func(ctx context.Context) ([]inspect.Socket, error)
	Kill  func(ctx context.Context, pid int32) (killer.Result, error)
	Open  func(url string) error
	Copy  func(text string) error
	User  string
	Every time.Duration // refresh interval
	// Now is the clock; StaleAfter how long a project's server runs before the
	// menu offers to stop it (default DefaultStaleAfter).
	Now        func() time.Time
	StaleAfter time.Duration
	// Monitors, if set, returns the uptime monitors to show (see MonitorViews).
	Monitors func() []MonitorView
}

// Tray keeps the menu in sync with the machine's ports.
type Tray struct {
	d    Deps
	menu Menu

	mu       sync.Mutex
	all      bool
	shown    string // signature of the menu on screen
	title    string // the menu bar title and tooltip on screen
	summary  string
	status   string // the last action's result, shown in the tooltip
	entries  []Entry
	stale    []Entry // entries running longer than StaleAfter, oldest first
	monitors []MonitorView
}

// New returns a tray drawing into menu.
func New(d Deps, menu Menu) *Tray { return &Tray{d: d, menu: menu} }

// Run refreshes the menu until ctx is done.
func (t *Tray) Run(ctx context.Context) {
	t.Refresh(ctx)
	tick := time.NewTicker(t.d.Every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.Refresh(ctx)
		}
	}
}

// Refresh lists the ports and redraws the menu if they changed.
func (t *Tray) Refresh(ctx context.Context) {
	sockets, err := t.d.List(ctx)
	var monitors []MonitorView
	if t.d.Monitors != nil {
		monitors = t.d.Monitors()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var entries, stale []Entry
	sig := "error:"
	if err == nil {
		entries = Entries(sockets, t.d.User, t.all)
		stale = t.staleEntries(entries)
		// Start times, not uptimes, go in the signature, so an open menu isn't
		// redrawn every minute; it changes when a server becomes stale.
		sig = fmt.Sprintf("%v %+v %d", t.all, entries, len(stale))
	} else {
		sig += err.Error()
	}
	// A monitor's state changes rarely (Since moves only when it does).
	sig += fmt.Sprintf(" %+v", monitors)
	if sig == t.shown {
		return
	}
	t.shown, t.entries, t.stale, t.monitors = sig, entries, stale, monitors
	t.draw(ctx, err)
}

// draw rebuilds the menu; t.mu is held.
func (t *Tray) draw(ctx context.Context, err error) {
	m := t.menu
	m.Reset()
	switch {
	case err != nil:
		t.title, t.summary = "!", err.Error()
	case len(t.entries) == 0:
		t.title, t.summary = "", "No ports listening"
	case len(t.entries) == 1:
		t.title, t.summary = "1", "1 port listening"
	default:
		t.title, t.summary = strconv.Itoa(len(t.entries)), fmt.Sprintf("%d ports listening", len(t.entries))
	}
	if _, down := monitorCounts(t.monitors); down > 0 {
		t.title = strings.TrimSpace(t.title + " ⚠" + strconv.Itoa(down))
		noun := "services"
		if down == 1 {
			noun = "service"
		}
		t.summary += fmt.Sprintf("; %d monitored %s down", down, noun)
	}
	m.Add(t.summary, "").Disable()
	m.SetTitle(t.title, t.tooltip())
	if len(t.stale) > 0 {
		t.addStale(ctx)
	}
	m.Separator()
	for _, e := range t.entries {
		t.addEntry(ctx, e)
	}
	if len(t.entries) > 0 {
		m.Separator()
	}
	if t.d.Monitors != nil {
		t.addMonitors(t.monitors)
		m.Separator()
	}
	label := "Show All Ports"
	if !t.all {
		label += " (also other users' and system)"
	}
	m.AddCheckbox(label, t.all).OnClick(func() {
		t.mu.Lock()
		t.all, t.shown = !t.all, ""
		t.mu.Unlock()
		t.Refresh(ctx)
	})
	m.Add("Refresh", "").OnClick(func() {
		t.mu.Lock()
		t.shown = ""
		t.mu.Unlock()
		t.Refresh(ctx)
	})
	m.Add("Quit portcheck", "").OnClick(m.Quit)
}

// tooltip describes the ports and the last action; t.mu is held.
func (t *Tray) tooltip() string {
	if t.status != "" {
		return "portcheck: " + t.summary + "\n" + t.status
	}
	return "portcheck: " + t.summary
}

// addEntry adds one port with its submenu; t.mu is held.
func (t *Tray) addEntry(ctx context.Context, e Entry) {
	item := t.menu.Add(e.Label(), e.StartedBy)
	switch {
	case e.PID != 0:
		item.Sub(fmt.Sprintf("Stop %s (pid %d)", e.Name, e.PID), "Asks it to exit, then kills it after 5 s. Never system processes.").
			OnClick(func() { t.stop(ctx, e) })
	case e.StopHint != "":
		item.Sub("Copy Stop Command", e.StopHint).OnClick(func() { t.copy(e.StopHint) })
	case e.Hidden:
		item.Sub("Owned by another user", "Run portcheck with sudo or as Administrator to see and stop it").Disable()
	}
	item.Sub("Open "+e.URL(), "").OnClick(func() { t.note(t.d.Open(e.URL()), "") })
	if e.PID != 0 {
		item.Sub("Copy PID", "").OnClick(func() { t.copy(strconv.Itoa(int(e.PID))) })
	}
	if e.StartedBy != "" {
		item.Sub("Started by "+e.StartedBy, "").Disable()
	}
	if !e.Started.IsZero() {
		item.Sub("Running since "+e.Started.Local().Format("Mon 2 Jan 15:04"), "").Disable()
	}
	if e.Cmdline != "" {
		cmd := e.Cmdline
		if len(cmd) > 60 {
			cmd = cmd[:57] + "..."
		}
		item.Sub(cmd, e.Cmdline).OnClick(func() { t.copy(e.Cmdline) })
	}
}

// staleEntries are the entries that look forgotten, oldest first, one per process.
func (t *Tray) staleEntries(entries []Entry) []Entry {
	now, after := time.Now(), cmp.Or(t.d.StaleAfter, DefaultStaleAfter)
	if t.d.Now != nil {
		now = t.d.Now()
	}
	var out []Entry
	for _, e := range entries {
		if e.Stale(now, after) && !slices.ContainsFunc(out, func(o Entry) bool { return o.PID == e.PID }) {
			out = append(out, e)
		}
	}
	slices.SortStableFunc(out, func(a, b Entry) int { return a.Started.Compare(b.Started) })
	return out
}

// addStale offers to stop the servers that have run longer than StaleAfter;
// t.mu is held.
func (t *Tray) addStale(ctx context.Context) {
	n := len(t.stale)
	since := "over a day"
	if days := cmp.Or(t.d.StaleAfter, DefaultStaleAfter) / (24 * time.Hour); days > 1 {
		since = fmt.Sprintf("over %d days", days)
	}
	title := fmt.Sprintf("%d dev servers running %s", n, since)
	label := fmt.Sprintf("Stop All %d", n)
	if n == 1 {
		title, label = "1 dev server running "+since, "Stop It"
	}
	item := t.menu.Add(title, "Processes in your projects that have been running a long time. You may have forgotten them.")
	stale := slices.Clone(t.stale)
	item.Sub(label, "Asks each to exit, then kills it after 5 s. Never system processes.").OnClick(func() {
		for _, e := range stale {
			t.stop(ctx, e)
		}
	})
	for _, e := range stale {
		item.Sub(e.Label()+"  (since "+e.Started.Local().Format("Mon 2 Jan")+")", e.Cmdline).
			OnClick(func() { t.stop(ctx, e) })
	}
}

func (t *Tray) stop(ctx context.Context, e Entry) {
	res, err := t.d.Kill(ctx, e.PID)
	msg := fmt.Sprintf("%s (pid %d): %s", e.Name, e.PID, res.Outcome)
	if err != nil {
		msg = fmt.Sprintf("couldn't stop %s (pid %d): %v", e.Name, e.PID, err)
	}
	t.mu.Lock()
	t.status, t.shown = msg, ""
	t.mu.Unlock()
	t.Refresh(ctx)
}

func (t *Tray) copy(text string) { t.note(t.d.Copy(text), "Copied: "+text) }

// note shows an action's result in the tooltip, without redrawing the menu.
func (t *Tray) note(err error, ok string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err != nil {
		t.status = err.Error()
	} else if ok != "" {
		t.status = ok
	}
	t.menu.SetTitle(t.title, t.tooltip())
}
