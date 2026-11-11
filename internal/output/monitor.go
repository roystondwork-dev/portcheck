package output

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/roystondwork-dev/portcheck/pkg/monitor"
)

// MonitorsJSON is the payload of `portcheck monitor list`.
type MonitorsJSON struct {
	File     string           `json:"file"`
	Monitors []monitor.Target `json:"monitors"`
}

// MonitorsDoc renders the monitors in the file at path.
func MonitorsDoc(path string, targets []monitor.Target) Document {
	d := Document{
		Kind:    "monitors",
		Data:    MonitorsJSON{File: path, Monitors: append([]monitor.Target{}, targets...)},
		Headers: []string{"NAME", "CHECKS", "EVERY", "UP WHEN"},
		Empty:   "No monitors yet. Add one: portcheck monitor add https://example.com/health",
	}
	for _, t := range targets {
		every, _ := t.Interval()
		d.Rows = append(d.Rows, []Cell{C(t.Name), C(t.Endpoint()), T(ShortDuration(every), ToneMuted), C(UpWhen(t))})
	}
	return d
}

// ShortDuration writes whole minutes and hours without zero parts: "1m",
// "2h", "1m30s".
func ShortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// UpWhen describes what counts as up, e.g. "HTTP 200-299, contains \"pong\"".
func UpWhen(t monitor.Target) string {
	if t.URL == "" {
		return "accepts connections"
	}
	s := "HTTP " + t.Status
	if t.Status == "" {
		s = "HTTP " + monitor.DefaultStatus
	}
	if t.Contains != "" {
		s += fmt.Sprintf(", contains %q", t.Contains)
	}
	return s
}

// MonitorCheckJSON is the payload of `portcheck monitor check`.
type MonitorCheckJSON struct {
	Results []monitor.Result `json:"results"`
}

// MonitorCheckDoc renders one round of checks.
func MonitorCheckDoc(results []monitor.Result) Document {
	d := Document{
		Kind:    "monitorCheck",
		Data:    MonitorCheckJSON{Results: append([]monitor.Result{}, results...)},
		Headers: []string{"NAME", "STATUS", "TIME", "DETAIL"},
		Empty:   "No monitors to check. Add one: portcheck monitor add https://example.com/health",
	}
	for _, r := range results {
		status := T("up", ToneGood)
		detail := r.Error
		if !r.Up {
			status = T("down", ToneBad)
		} else if r.Status != 0 {
			detail = "HTTP " + strconv.Itoa(r.Status)
		}
		d.Rows = append(d.Rows, []Cell{C(r.Name), status, T(strconv.FormatFloat(r.Latency, 'f', 0, 64)+" ms", ToneMuted), C(detail)})
	}
	return d
}
