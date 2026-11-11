package tray

import (
	"fmt"
	"strings"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/monitor"
)

// MonitorView is one monitor in the menu.
type MonitorView struct {
	Name     string
	Endpoint string
	IsURL    bool
	Status   monitor.Status
	Error    string    // why it's down
	Since    time.Time // when it went down (or up)
}

// MonitorViews joins the running monitors with what's known about them, in
// file order.
func MonitorViews(targets []monitor.Target, states map[string]monitor.State) []MonitorView {
	out := make([]MonitorView, 0, len(targets))
	for _, t := range targets {
		v := MonitorView{Name: t.Name, Endpoint: t.Endpoint(), IsURL: t.URL != "", Status: monitor.Unknown}
		if s, ok := states[t.Name]; ok {
			v.Status, v.Since = s.Status, s.Since
			if s.Status == monitor.Down {
				v.Error = s.Last.Error
			}
		}
		out = append(out, v)
	}
	return out
}

// Label is the menu item's title, e.g. "✓ example.com — up" or
// "✗ api — down: timed out".
func (v MonitorView) Label() string {
	switch v.Status {
	case monitor.Up:
		return "✓ " + v.Name + " — up"
	case monitor.Down:
		return "✗ " + v.Name + " — down: " + v.Error
	default:
		return "… " + v.Name + " — checking"
	}
}

// monitorCounts says how many monitors are up and down.
func monitorCounts(views []MonitorView) (up, down int) {
	for _, v := range views {
		switch v.Status {
		case monitor.Up:
			up++
		case monitor.Down:
			down++
		case monitor.Unknown:
		}
	}
	return up, down
}

// monitorSummary is the section's heading, e.g. "Monitors: 2 up, 1 down".
func monitorSummary(views []MonitorView) string {
	if len(views) == 0 {
		return "Monitors: none (add one: portcheck monitor add <url>)"
	}
	up, down := monitorCounts(views)
	parts := []string{fmt.Sprintf("%d up", up)}
	if down > 0 {
		parts = append(parts, fmt.Sprintf("%d down", down))
	}
	if n := len(views) - up - down; n > 0 {
		parts = append(parts, fmt.Sprintf("%d checking", n))
	}
	return "Monitors: " + strings.Join(parts, ", ")
}

// addMonitors adds the monitors section; t.mu is held.
func (t *Tray) addMonitors(views []MonitorView) {
	t.menu.Add(monitorSummary(views), "Checked on this device while it's on. Manage with portcheck monitor add/remove.").Disable()
	for _, v := range views {
		tip := v.Endpoint
		if !v.Since.IsZero() && v.Status != monitor.Unknown {
			tip += fmt.Sprintf(" — %s since %s", v.Status, v.Since.Local().Format("Mon 15:04"))
		}
		item := t.menu.Add(v.Label(), tip)
		if v.IsURL {
			item.Sub("Open "+v.Endpoint, "").OnClick(func() { t.note(t.d.Open(v.Endpoint), "") })
		}
		item.Sub("Copy "+v.Endpoint, "").OnClick(func() { t.copy(v.Endpoint) })
		if !v.Since.IsZero() && v.Status != monitor.Unknown {
			item.Sub(fmt.Sprintf("%s since %s", strings.ToUpper(string(v.Status[:1]))+string(v.Status[1:]), v.Since.Local().Format("Mon 2 Jan 15:04")), "").Disable()
		}
	}
}
