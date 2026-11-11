package monitor

import (
	"strings"
	"sync"
	"time"
)

// Status is a monitor's state.
type Status string

// The states of a monitor.
const (
	Unknown Status = "unknown" // not checked yet, or failing but under the threshold
	Up      Status = "up"
	Down    Status = "down"
)

// DefaultThreshold is how many failed checks in a row mark a monitor down,
// so one dropped request doesn't raise an alarm.
const DefaultThreshold = 2

// State is what's known about one monitor.
type State struct {
	Name   string    `json:"name"`
	Status Status    `json:"status"`
	Since  time.Time `json:"since"` // when Status began; zero while unknown
	Fails  int       `json:"fails"` // failed checks in a row
	Last   Result    `json:"last"`
	// failingSince is when the current run of failures began.
	failingSince time.Time
}

// Event is a change worth telling the user about.
type Event struct {
	Name     string
	Endpoint string
	Status   Status // Down or Up
	// Reason says why it's down (Down events).
	Reason string
	// DownFor is how long it was down (Up events after a Down).
	DownFor time.Duration
	At      time.Time
}

// Message is the event in a sentence, e.g. "royston.in is down: timed out"
// or "royston.in is back up after 4m".
func (e Event) Message() string {
	if e.Status == Down {
		return e.Name + " is down: " + e.Reason
	}
	return e.Name + " is back up after " + roundDuration(e.DownFor)
}

// roundDuration writes d to the second, minute or hour: "45s", "4m", "3h".
func roundDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return d.Round(time.Second).String()
	case d < time.Hour:
		return strings.TrimSuffix(d.Round(time.Minute).String(), "0s")
	default:
		return strings.TrimSuffix(d.Round(time.Hour).String(), "0m0s")
	}
}

// Tracker turns check results into states and events. It's safe for
// concurrent use.
type Tracker struct {
	// Threshold is how many failures in a row mark a monitor down; values
	// below 1 mean DefaultThreshold.
	Threshold int
	mu        sync.Mutex
	states    map[string]*State
}

// Observe records a result and returns the event it causes, if any: Down
// when the failures reach the threshold, Up when a down monitor answers.
func (t *Tracker) Observe(r Result) (Event, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.states == nil {
		t.states = map[string]*State{}
	}
	s, ok := t.states[r.Name]
	if !ok {
		s = &State{Name: r.Name, Status: Unknown}
		t.states[r.Name] = s
	}
	s.Last = r
	threshold := t.Threshold
	if threshold < 1 {
		threshold = DefaultThreshold
	}
	if r.Up {
		s.Fails, s.failingSince = 0, time.Time{}
		was, since := s.Status, s.Since
		if was != Up {
			s.Status, s.Since = Up, r.At
		}
		if was == Down {
			return Event{Name: r.Name, Endpoint: r.Endpoint, Status: Up, DownFor: r.At.Sub(since), At: r.At}, true
		}
		return Event{}, false
	}
	s.Fails++
	if s.Fails == 1 {
		s.failingSince = r.At
	}
	if s.Fails < threshold || s.Status == Down {
		return Event{}, false
	}
	// Down since the first of these failures.
	s.Status, s.Since = Down, s.failingSince
	return Event{Name: r.Name, Endpoint: r.Endpoint, Status: Down, Reason: r.Error, At: r.At}, true
}

// States returns a copy of every monitor's state.
func (t *Tracker) States() map[string]State {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]State, len(t.states))
	for k, v := range t.states {
		out[k] = *v
	}
	return out
}

// Forget drops monitors that are no longer in names (e.g. removed from the file).
func (t *Tracker) Forget(names map[string]bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k := range t.states {
		if !names[k] {
			delete(t.states, k)
		}
	}
}
