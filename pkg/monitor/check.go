package monitor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Result is the outcome of one check.
type Result struct {
	Name     string    `json:"name"`
	Endpoint string    `json:"endpoint"`
	Up       bool      `json:"up"`
	Status   int       `json:"status,omitempty"` // HTTP status, for URLs that answered
	Latency  float64   `json:"latencyMs"`        // how long the check took
	Error    string    `json:"error,omitempty"`  // why it's down, in plain words
	At       time.Time `json:"at"`
}

// maxBody is how much of a response is read to look for Contains.
const maxBody = 1 << 20

// Dialer opens TCP connections; *net.Dialer is one.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Checker runs checks.
type Checker struct {
	// Client sends HTTP requests; nil means a client that follows redirects
	// and keeps no connections between checks.
	Client *http.Client
	Dialer Dialer
	// UserAgent is sent with HTTP requests.
	UserAgent string
	Now       func() time.Time
}

// NewChecker returns a Checker with real networking.
func NewChecker(userAgent string) *Checker {
	return &Checker{
		Client:    &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DisableKeepAlives: true}},
		Dialer:    &net.Dialer{},
		UserAgent: userAgent,
		Now:       time.Now,
	}
}

// Check checks t once. t must be valid (see Target.Validate).
func (c *Checker) Check(ctx context.Context, t Target) Result {
	timeout, _ := t.TimeoutDuration()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := c.Now()
	r := Result{Name: t.Name, Endpoint: t.Endpoint(), At: start}
	if t.URL != "" {
		r.Status, r.Error = c.checkURL(ctx, t)
	} else {
		r.Error = c.checkTCP(ctx, t)
	}
	if r.Error != "" && ctx.Err() != nil {
		r.Error = fmt.Sprintf("no answer within %s", timeout)
	}
	r.Up = r.Error == ""
	r.Latency = float64(c.Now().Sub(start).Microseconds()) / 1000
	return r
}

func (c *Checker) checkTCP(ctx context.Context, t Target) string {
	conn, err := c.Dialer.DialContext(ctx, "tcp", t.Address)
	if err != nil {
		return describe(err)
	}
	_ = conn.Close()
	return ""
}

func (c *Checker) checkURL(ctx context.Context, t Target) (int, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, http.NoBody)
	if err != nil {
		return 0, describe(err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.Client.Do(req)
	if err != nil {
		return 0, describe(err)
	}
	defer func() { _ = resp.Body.Close() }()
	statuses, _ := t.Statuses()
	if !statuses.Has(resp.StatusCode) {
		return resp.StatusCode, fmt.Sprintf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	if t.Contains == "" {
		return resp.StatusCode, ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.StatusCode, describe(err)
	}
	if !bytes.Contains(body, []byte(t.Contains)) {
		return resp.StatusCode, fmt.Sprintf("the response doesn't contain %q", t.Contains)
	}
	return resp.StatusCode, ""
}

// describe turns a network error into a short reason a person understands.
func describe(err error) string {
	var dnsErr *net.DNSError
	switch {
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return "host not found: " + dnsErr.Name
	case isAny(err, errRefused):
		return "connection refused"
	case isAny(err, errReset):
		return "connection reset"
	case isAny(err, errUnreachable):
		return "host unreachable"
	case errors.Is(err, os.ErrDeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case strings.Contains(err.Error(), "certificate"):
		return "TLS certificate problem: " + lastPart(err.Error())
	default:
		return lastPart(err.Error())
	}
}

func isAny(err error, targets []error) bool {
	for _, t := range targets {
		if errors.Is(err, t) {
			return true
		}
	}
	return false
}

// lastPart drops Go's "Get "https://…": dial tcp …:" prefixes.
func lastPart(s string) string {
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}
