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
	StopHint  string // for containers: the command that stops them
	Hidden    bool   // owner not visible to this user
}

// URL opens the port in a browser.
func (e Entry) URL() string { return fmt.Sprintf("http://localhost:%d/", e.Port) }

// Label is the menu item's title, e.g. ":3000  node".
func (e Entry) Label() string { return fmt.Sprintf(":%d  %s", e.Port, e.Name) }

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
				e.Name, e.Cmdline = p.Name, p.Cmdline
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
}

// Tray keeps the menu in sync with the machine's ports.
type Tray struct {
	d    Deps
	menu Menu

	mu      sync.Mutex
	all     bool
	shown   string // signature of the menu on screen
	title   string // the menu bar title and tooltip on screen
	summary string
	status  string // the last action's result, shown in the tooltip
	entries []Entry
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
	t.mu.Lock()
	defer t.mu.Unlock()
	var entries []Entry
	sig := "error:"
	if err == nil {
		entries = Entries(sockets, t.d.User, t.all)
		sig = fmt.Sprintf("%v %+v", t.all, entries)
	} else {
		sig += err.Error()
	}
	if sig == t.shown {
		return
	}
	t.shown, t.entries = sig, entries
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
	m.Add(t.summary, "").Disable()
	m.SetTitle(t.title, t.tooltip())
	m.Separator()
	for _, e := range t.entries {
		t.addEntry(ctx, e)
	}
	if len(t.entries) > 0 {
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
	if e.Cmdline != "" {
		cmd := e.Cmdline
		if len(cmd) > 60 {
			cmd = cmd[:57] + "..."
		}
		item.Sub(cmd, e.Cmdline).OnClick(func() { t.copy(e.Cmdline) })
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
