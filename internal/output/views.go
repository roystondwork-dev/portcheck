package output

import (
	"cmp"
	"strconv"
	"strings"
	"time"

	"github.com/roystondwork-dev/portcheck/internal/buildinfo"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/scan"
)

// JSON payload types. Field names and meanings are part of the public,
// versioned output schema (see SchemaVersion and docs/json-schema.md).

// SocketJSON is one socket.
type SocketJSON struct {
	Proto         string         `json:"proto"`
	LocalAddress  string         `json:"localAddress"`
	LocalPort     uint16         `json:"localPort"`
	RemoteAddress string         `json:"remoteAddress,omitempty"`
	RemotePort    uint16         `json:"remotePort,omitempty"`
	State         string         `json:"state"`
	PID           int32          `json:"pid,omitempty"`
	Process       *ProcessJSON   `json:"process,omitempty"`
	Container     *ContainerJSON `json:"container,omitempty"`
}

// ContainerJSON is the container publishing a socket's port.
type ContainerJSON struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Image   string `json:"image,omitempty"`
	Runtime string `json:"runtime"`
}

// ProcessJSON is a socket's owning process.
type ProcessJSON struct {
	PID       int32            `json:"pid"`
	Name      string           `json:"name"`
	User      string           `json:"user,omitempty"`
	Cmdline   string           `json:"cmdline,omitempty"`
	Exe       string           `json:"exe,omitempty"`
	StartTime *time.Time       `json:"startTime,omitempty"`
	Ancestors []ProcessRefJSON `json:"ancestors,omitempty"`
}

// ProcessRefJSON identifies an ancestor process.
type ProcessRefJSON struct {
	PID  int32  `json:"pid"`
	Name string `json:"name"`
}

// ListJSON is the payload of `portcheck list`.
type ListJSON struct {
	Sockets []SocketJSON `json:"sockets"`
	// Partial is true when some owners could not be determined (permissions).
	Partial bool `json:"partial"`
}

// PortStatusJSON is the status of one checked port.
type PortStatusJSON struct {
	Port    uint16       `json:"port"`
	Proto   string       `json:"proto"`
	Status  string       `json:"status"` // free | in-use | denied
	Holders []SocketJSON `json:"holders"`
}

// CheckJSON is the payload of `portcheck check`.
type CheckJSON struct {
	Ports []PortStatusJSON `json:"ports"`
}

// FreeJSON is the payload of `portcheck free`.
type FreeJSON struct {
	Ports []uint16 `json:"ports"`
}

// KillResultJSON is the result for one process.
type KillResultJSON struct {
	PID     int32  `json:"pid"`
	Name    string `json:"name,omitempty"`
	Outcome string `json:"outcome"` // terminated | killed | already-exited | dry-run | failed
	Error   string `json:"error,omitempty"`
}

// KillJSON is the payload of `portcheck kill`.
type KillJSON struct {
	Results []KillResultJSON `json:"results"`
}

func socketJSON(s inspect.Socket) SocketJSON {
	out := SocketJSON{
		Proto:        string(s.Proto),
		LocalAddress: s.Local.Addr().String(),
		LocalPort:    s.Local.Port(),
		State:        string(s.State),
		PID:          s.PID,
	}
	if s.Remote.IsValid() {
		out.RemoteAddress = s.Remote.Addr().String()
		out.RemotePort = s.Remote.Port()
	}
	if p := s.Process; p != nil {
		pj := &ProcessJSON{PID: p.PID, Name: p.Name, User: p.User, Cmdline: p.Cmdline, Exe: p.Exe}
		for _, a := range p.Ancestors {
			pj.Ancestors = append(pj.Ancestors, ProcessRefJSON{PID: a.PID, Name: a.Name})
		}
		if !p.CreateTime.IsZero() {
			t := p.CreateTime.UTC()
			pj.StartTime = &t
		}
		out.Process = pj
	}
	if c := s.Container; c != nil {
		out.Container = &ContainerJSON{ID: c.ID, Name: c.Name, Image: c.Image, Runtime: c.Runtime}
	}
	return out
}

func socketsJSON(ss []inspect.Socket) []SocketJSON {
	out := make([]SocketJSON, len(ss))
	for i, s := range ss {
		out[i] = socketJSON(s)
	}
	return out
}

func pidCell(pid int32) Cell {
	if pid <= 0 {
		return T("-", ToneMuted)
	}
	return C(strconv.Itoa(int(pid)))
}

// ContainerLabel names a container for display, e.g. "docker:web".
func ContainerLabel(c *inspect.Container) string {
	return c.Runtime + ":" + cmp.Or(c.Name, c.Image, c.ID)
}

// processCells returns the PROCESS and USER cells. Ports published by a
// container show the container instead of the proxy process holding them.
func processCells(s inspect.Socket) (name, user Cell) {
	user = T("?", ToneMuted)
	if p := s.Process; p != nil {
		user = C(cmp.Or(p.User, "-"))
	}
	switch {
	case s.Container != nil:
		return T(ContainerLabel(s.Container), ToneInfo), user
	case s.Process != nil:
		return C(s.Process.Name), user
	default:
		return T("?", ToneMuted), user
	}
}

func stateTone(s inspect.State) Tone {
	switch s {
	case inspect.StateListen:
		return ToneGood
	case inspect.StateEstablished:
		return ToneNone
	default:
		return ToneMuted
	}
}

// Ancestry describes who started p, nearest first, e.g. "npm (4200) ← zsh (900)".
// It returns "" when unknown.
func Ancestry(p *inspect.ProcessInfo) string {
	if p == nil {
		return ""
	}
	parts := make([]string, len(p.Ancestors))
	for i, a := range p.Ancestors {
		parts[i] = a.Name + " (" + strconv.Itoa(int(a.PID)) + ")"
	}
	return strings.Join(parts, " ← ")
}

// maxCommandWidth bounds the COMMAND column of `list --wide`.
const maxCommandWidth = 60

// Truncate shortens s to at most n runes, ending with "…" if it was cut.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// ListDoc renders the result of `portcheck list`. wide adds the PARENT and
// COMMAND columns.
func ListDoc(res *inspect.Result, wide bool) Document {
	d := Document{
		Kind:    "list",
		Data:    ListJSON{Sockets: socketsJSON(res.Sockets), Partial: res.Partial},
		Headers: []string{"PROTO", "LOCAL ADDRESS", "REMOTE ADDRESS", "STATE", "PID", "PROCESS", "USER"},
		Empty:   "No matching sockets.",
	}
	if wide {
		d.Headers = append(d.Headers, "PARENT", "COMMAND")
	}
	for _, s := range res.Sockets {
		remote := T("-", ToneMuted)
		if s.Remote.IsValid() {
			remote = C(s.Remote.String())
		}
		name, user := processCells(s)
		row := []Cell{
			C(string(s.Proto)), C(s.Local.String()), remote,
			T(string(s.State), stateTone(s.State)), pidCell(s.PID), name, user,
		}
		if wide {
			parent, command := T("-", ToneMuted), T("-", ToneMuted)
			if p := s.Process.Parent(); p != nil {
				parent = C(p.Name + " (" + strconv.Itoa(int(p.PID)) + ")")
			}
			if s.Process != nil && s.Process.Cmdline != "" {
				command = C(Truncate(s.Process.Cmdline, maxCommandWidth))
			}
			row = append(row, parent, command)
		}
		d.Rows = append(d.Rows, row)
	}
	return d
}

func availabilityTone(a freeport.Availability) Tone {
	switch a {
	case freeport.Free:
		return ToneGood
	case freeport.InUse:
		return ToneBad
	default:
		return ToneWarn
	}
}

// CheckDoc renders the result of `portcheck check`.
func CheckDoc(sts []freeport.Status) Document {
	d := Document{
		Kind:    "check",
		Headers: []string{"PORT", "PROTO", "STATUS", "PID", "PROCESS", "USER"},
	}
	data := CheckJSON{Ports: make([]PortStatusJSON, len(sts))}
	for i, st := range sts {
		data.Ports[i] = PortStatusJSON{Port: st.Port, Proto: string(st.Proto), Status: st.Availability.String(), Holders: socketsJSON(st.Holders)}
		status := T(st.Availability.String(), availabilityTone(st.Availability))
		port, proto := C(strconv.Itoa(int(st.Port))), C(string(st.Proto))
		if len(st.Holders) == 0 {
			// In use but owner unknown (permissions) still gets a row.
			d.Rows = append(d.Rows, []Cell{port, proto, status, T("-", ToneMuted), T("-", ToneMuted), T("-", ToneMuted)})
			continue
		}
		for _, h := range st.Holders {
			name, user := processCells(h)
			d.Rows = append(d.Rows, []Cell{port, proto, status, pidCell(h.PID), name, user})
		}
	}
	d.Data = data
	return d
}

// FreeDoc renders the result of `portcheck free`.
func FreeDoc(ports []uint16) Document {
	d := Document{
		Kind:    "free",
		Data:    FreeJSON{Ports: append([]uint16{}, ports...)},
		Headers: []string{"PORT"},
		Empty:   "No free port found.",
	}
	for _, p := range ports {
		d.Rows = append(d.Rows, []Cell{T(strconv.Itoa(int(p)), ToneGood)})
	}
	return d
}

func outcomeTone(outcome string) Tone {
	switch outcome {
	case "terminated", "killed":
		return ToneGood
	case "failed":
		return ToneBad
	default:
		return ToneMuted
	}
}

// KillDoc renders the result of `portcheck kill`.
func KillDoc(results []KillResultJSON) Document {
	d := Document{
		Kind:    "kill",
		Data:    KillJSON{Results: append([]KillResultJSON{}, results...)},
		Headers: []string{"PID", "PROCESS", "RESULT", "DETAIL"},
		Empty:   "Nothing to kill.",
	}
	for _, r := range results {
		d.Rows = append(d.Rows, []Cell{
			pidCell(r.PID), C(cmp.Or(r.Name, "?")), T(r.Outcome, outcomeTone(r.Outcome)), C(r.Error),
		})
	}
	return d
}

// VersionDoc renders build information.
func VersionDoc(info buildinfo.Info) Document {
	return Document{
		Kind:    "version",
		Data:    info,
		Text:    info.String(),
		Headers: []string{"VERSION", "COMMIT", "DATE", "MODIFIED", "GO", "PLATFORM"},
		Rows: [][]Cell{{
			C(info.Version), C(info.Commit), C(info.Date), C(strconv.FormatBool(info.Modified)), C(info.GoVersion), C(info.Platform),
		}},
	}
}

// ScanResultJSON is one probed host/port.
type ScanResultJSON struct {
	Host      string  `json:"host"`
	Address   string  `json:"address"`
	Port      uint16  `json:"port"`
	State     string  `json:"state"` // open | closed | filtered | error
	Service   string  `json:"service,omitempty"`
	LatencyMs float64 `json:"latencyMs"`
	Error     string  `json:"error,omitempty"`
}

// ScanSummaryJSON totals a scan.
type ScanSummaryJSON struct {
	Hosts      int   `json:"hosts"`
	Ports      int   `json:"ports"`
	Open       int   `json:"open"`
	Closed     int   `json:"closed"`
	Filtered   int   `json:"filtered"`
	Errors     int   `json:"errors"`
	DurationMs int64 `json:"durationMs"`
}

// ScanJSON is the payload of `portcheck scan`.
type ScanJSON struct {
	Results []ScanResultJSON `json:"results"`
	Summary ScanSummaryJSON  `json:"summary"`
}

func scanStateTone(s scan.State) Tone {
	switch s {
	case scan.Open:
		return ToneGood
	case scan.Filtered:
		return ToneWarn
	case scan.Error:
		return ToneBad
	default:
		return ToneMuted
	}
}

// ScanDoc renders the result of `portcheck scan`. Results are shown in the
// order given.
func ScanDoc(results []scan.Result, sum scan.Summary, took time.Duration) Document {
	data := ScanJSON{
		Results: make([]ScanResultJSON, len(results)),
		Summary: ScanSummaryJSON{
			Hosts: sum.Hosts, Ports: sum.Ports, Open: sum.Open, Closed: sum.Closed,
			Filtered: sum.Filtered, Errors: sum.Errored, DurationMs: took.Milliseconds(),
		},
	}
	d := Document{
		Kind:    "scan",
		Headers: []string{"HOST", "PORT", "STATE", "SERVICE", "LATENCY", "DETAIL"},
		Empty:   "No results to show.",
	}
	for i, r := range results {
		rj := ScanResultJSON{
			Host: r.Target.Host, Address: r.Target.Addr.String(), Port: r.Port, State: string(r.State),
			Service: r.Service, LatencyMs: float64(r.Latency.Microseconds()) / 1000,
		}
		if r.Err != nil {
			rj.Error = r.Err.Error()
		}
		data.Results[i] = rj
		d.Rows = append(d.Rows, []Cell{
			C(r.Target.String()), C(strconv.Itoa(int(r.Port))), T(string(r.State), scanStateTone(r.State)),
			C(cmp.Or(r.Service, "-")), T(r.Latency.Round(100*time.Microsecond).String(), ToneMuted), C(rj.Error),
		})
	}
	d.Data = data
	return d
}

// WaitResultJSON is the outcome for one awaited target.
type WaitResultJSON struct {
	Target    string `json:"target"` // as given on the command line
	Host      string `json:"host"`
	Port      uint16 `json:"port"`
	Condition string `json:"condition"` // open | free
	Ready     bool   `json:"ready"`
	WaitedMs  int64  `json:"waitedMs"`
	Attempts  int    `json:"attempts"`
}

// WaitJSON is the payload of `portcheck wait`.
type WaitJSON struct {
	Targets []WaitResultJSON `json:"targets"`
}

// WaitDoc renders the result of `portcheck wait`.
func WaitDoc(results []WaitResultJSON) Document {
	d := Document{
		Kind:    "wait",
		Data:    WaitJSON{Targets: append([]WaitResultJSON{}, results...)},
		Headers: []string{"TARGET", "CONDITION", "STATUS", "WAITED", "ATTEMPTS"},
	}
	for _, r := range results {
		status := T("ready", ToneGood)
		if !r.Ready {
			status = T("timeout", ToneBad)
		}
		waited := (time.Duration(r.WaitedMs) * time.Millisecond).String()
		d.Rows = append(d.Rows, []Cell{
			C(r.Target), C(r.Condition), status, T(waited, ToneMuted), C(strconv.Itoa(r.Attempts)),
		})
	}
	return d
}
