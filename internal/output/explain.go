package output

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
)

// CandidateJSON is a port a command line probably listens on.
type CandidateJSON struct {
	Port uint16 `json:"port"`
	// Source is "explicit" (on the command line or in an environment variable)
	// or "default" (a known tool's default port).
	Source string `json:"source"`
}

// ExplainJSON is the payload of `portcheck explain`.
type ExplainJSON struct {
	Command  string          `json:"command"`
	ExitCode int             `json:"exitCode"`
	Ports    []CandidateJSON `json:"ports"`
	// InUse lists the candidate ports something else holds, as in `check`.
	InUse []PortStatusJSON `json:"inUse"`
}

// ExplainDoc renders `portcheck explain`. lines are the human-readable
// explanation; with none, the table and plain formats print nothing.
func ExplainDoc(data ExplainJSON, sts []freeport.Status, lines []string) Document {
	data.InUse = make([]PortStatusJSON, len(sts))
	for i, st := range sts {
		data.InUse[i] = PortStatusJSON{Port: st.Port, Proto: string(st.Proto), Status: st.Availability.String(), Holders: socketsJSON(st.Holders)}
	}
	if data.Ports == nil {
		data.Ports = []CandidateJSON{}
	}
	return Document{Kind: "explain", Data: data, Text: strings.Join(lines, "\n")}
}

// ExplainLines says who holds a port in two lines: who, and how to free it.
// runHint, if set, is a command that starts the failed command on another
// port instead.
func ExplainLines(st freeport.Status, runHint string) []string {
	port := strconv.Itoa(int(st.Port))
	who, fix := "another user's process", "see who: sudo portcheck "+port
	more := 0
	for i, h := range uniqueHolders(st.Holders) {
		if i > 0 {
			more++
			continue
		}
		switch {
		case h.Container != nil:
			who, fix = DescribeContainer(h.Container), "stop it: "+StopHint(h.Container)
		case h.PID > 0:
			who = fmt.Sprintf("%s (pid %d)", processName(h), h.PID)
			if by := Ancestry(h.Process); by != "" {
				who += ", started by " + by
			}
			if h.Process != nil && h.Process.Project != "" {
				who += ", in " + h.Process.Project
			}
			fix = "stop it: portcheck kill " + port
			if isAirPlay(h) {
				// macOS restarts it when stopped.
				who = "macOS AirPlay Receiver (ControlCenter, pid " + strconv.Itoa(int(h.PID)) + ")"
				fix = "turn it off in System Settings → General → AirDrop & Handoff → AirPlay Receiver, or use another port"
			}
			if runHint != "" {
				fix += "  ·  or use a free port: " + runHint
			}
		}
	}
	verb := "held by "
	if strings.HasPrefix(who, "published by") || strings.HasPrefix(who, "forwarded from") {
		verb = ""
	}
	line := "portcheck: port " + port + " is " + verb + who
	if more > 0 {
		line += fmt.Sprintf(" (and %d more)", more)
	}
	return []string{line, "portcheck: " + fix}
}

// isAirPlay reports whether s is macOS's AirPlay Receiver, which listens on
// ports 5000 and 7000 and comes back if it is stopped.
func isAirPlay(s inspect.Socket) bool {
	return s.Process != nil && s.Process.Name == "ControlCenter" && (s.Port() == 5000 || s.Port() == 7000)
}

func processName(s inspect.Socket) string {
	if s.Process != nil && s.Process.Name != "" {
		return s.Process.Name
	}
	return "a process"
}

// uniqueHolders keeps one socket per process or container (IPv4 and IPv6
// listeners of one process are one holder).
func uniqueHolders(ss []inspect.Socket) []inspect.Socket {
	var out []inspect.Socket
	seen := map[string]bool{}
	for _, s := range ss {
		key := strconv.Itoa(int(s.PID))
		if s.Container != nil {
			key = "c:" + ContainerLabel(s.Container)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, s)
		}
	}
	return out
}
