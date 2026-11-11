// Package monitor checks that services stay up: URLs that must answer with a
// good HTTP status, and TCP ports that must accept connections. It runs on
// the user's own device, on a schedule, and reports when a service goes down
// and when it comes back. Nothing is hosted: checks stop when the device
// sleeps or shuts down.
//
// A [File] holds the user's monitors; [Checker] runs one check; a [Tracker]
// turns results into down and recovered [Event]s, with a failure threshold so
// one dropped request doesn't raise an alarm; a [Runner] schedules the checks.
package monitor

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Defaults and limits for a monitor's schedule.
const (
	DefaultEvery   = time.Minute
	DefaultTimeout = 10 * time.Second
	// MinEvery keeps a monitor from hammering a service.
	MinEvery = 10 * time.Second
	// DefaultStatus is what counts as up for a URL, after redirects.
	DefaultStatus = "200-299"
)

// Target is one monitor: a URL or a TCP address, and how to check it.
type Target struct {
	// Name identifies the monitor in lists and alerts, e.g. "royston.in/ping".
	Name string `yaml:"name" json:"name"`
	// URL is an http or https URL; Address is host:port for a TCP check.
	// Exactly one is set.
	URL     string `yaml:"url,omitempty" json:"url,omitempty"`
	Address string `yaml:"address,omitempty" json:"address,omitempty"`
	// Every is how often to check, e.g. "1m"; empty means DefaultEvery.
	Every string `yaml:"every,omitempty" json:"every,omitempty"`
	// Timeout bounds one check, e.g. "10s"; empty means DefaultTimeout.
	Timeout string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	// Status lists the HTTP statuses that count as up, e.g. "200-399" or
	// "200,204"; empty means DefaultStatus. URLs only.
	Status string `yaml:"status,omitempty" json:"status,omitempty"`
	// Contains, if set, must appear in the response body. URLs only.
	Contains string `yaml:"contains,omitempty" json:"contains,omitempty"`
}

// ParseTarget reads a command-line target: an http(s) URL, host:port, or a
// bare port on localhost. The name defaults to the URL without its scheme,
// or the address.
func ParseTarget(arg string) (Target, error) {
	arg = strings.TrimSpace(arg)
	if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
		name := strings.TrimSuffix(strings.SplitN(arg, "://", 2)[1], "/")
		if name == "" {
			name = arg
		}
		t := Target{URL: arg, Name: name}
		return t, t.Validate()
	}
	if strings.Contains(arg, "://") {
		return Target{}, fmt.Errorf("%q: only http:// and https:// URLs can be monitored", arg)
	}
	addr := arg
	if _, err := strconv.ParseUint(arg, 10, 16); err == nil {
		addr = net.JoinHostPort("localhost", arg)
	}
	t := Target{Address: addr, Name: addr}
	return t, t.Validate()
}

// Validate reports what's wrong with t, if anything.
func (t Target) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("a monitor needs a name")
	}
	switch {
	case t.URL != "" && t.Address != "":
		return fmt.Errorf("monitor %q: set url or address, not both", t.Name)
	case t.URL != "":
		u, err := url.Parse(t.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("monitor %q: %q is not an http or https URL", t.Name, t.URL)
		}
	case t.Address != "":
		host, port, err := net.SplitHostPort(t.Address)
		if err != nil || host == "" {
			return fmt.Errorf("monitor %q: %q is not host:port", t.Name, t.Address)
		}
		if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
			return fmt.Errorf("monitor %q: %q is not a port (1-65535)", t.Name, port)
		}
		if t.Status != "" || t.Contains != "" {
			return fmt.Errorf("monitor %q: status and contains only apply to URLs", t.Name)
		}
	default:
		return fmt.Errorf("monitor %q: set a url or an address", t.Name)
	}
	every, err := t.Interval()
	if err != nil {
		return err
	}
	timeout, err := t.TimeoutDuration()
	if err != nil {
		return err
	}
	if timeout >= every {
		return fmt.Errorf("monitor %q: timeout (%s) must be shorter than every (%s)", t.Name, timeout, every)
	}
	if _, err := t.Statuses(); err != nil {
		return err
	}
	return nil
}

// Endpoint is what the monitor checks: its URL or its address.
func (t Target) Endpoint() string {
	if t.URL != "" {
		return t.URL
	}
	return t.Address
}

// Interval is how often to check.
func (t Target) Interval() (time.Duration, error) {
	d, err := duration(t.Every, DefaultEvery)
	if err != nil {
		return 0, fmt.Errorf("monitor %q: every: %w", t.Name, err)
	}
	if d < MinEvery {
		return 0, fmt.Errorf("monitor %q: every must be at least %s", t.Name, MinEvery)
	}
	return d, nil
}

// TimeoutDuration bounds one check.
func (t Target) TimeoutDuration() (time.Duration, error) {
	d, err := duration(t.Timeout, DefaultTimeout)
	if err != nil {
		return 0, fmt.Errorf("monitor %q: timeout: %w", t.Name, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("monitor %q: timeout must be positive", t.Name)
	}
	return d, nil
}

func duration(s string, def time.Duration) (time.Duration, error) {
	if s == "" {
		return def, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration like 30s or 5m", s)
	}
	return d, nil
}

// StatusSet is the HTTP statuses that count as up.
type StatusSet []statusRange

type statusRange struct{ from, to int }

// Has reports whether code is in the set.
func (s StatusSet) Has(code int) bool {
	for _, r := range s {
		if code >= r.from && code <= r.to {
			return true
		}
	}
	return false
}

// Statuses parses t.Status, e.g. "200-399" or "200,204".
func (t Target) Statuses() (StatusSet, error) {
	spec := t.Status
	if spec == "" {
		spec = DefaultStatus
	}
	var set StatusSet
	for part := range strings.SplitSeq(spec, ",") {
		from, to, isRange := strings.Cut(strings.TrimSpace(part), "-")
		if !isRange {
			to = from
		}
		a, errA := strconv.Atoi(from)
		b, errB := strconv.Atoi(to)
		if errA != nil || errB != nil || a < 100 || b > 599 || b < a {
			return nil, fmt.Errorf("monitor %q: status %q is not like 200-299 or 200,204", t.Name, t.Status)
		}
		set = append(set, statusRange{a, b})
	}
	return set, nil
}
