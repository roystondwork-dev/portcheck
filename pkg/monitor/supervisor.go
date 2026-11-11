package monitor

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"
)

// DefaultReload is how often a Supervisor looks for changes to its file.
const DefaultReload = 15 * time.Second

// Supervisor runs the monitors in a file and picks up changes to the file
// while running, keeping what's known about monitors that stay.
type Supervisor struct {
	Path   string
	Runner *Runner
	// Reload is how often to look for changes; 0 means DefaultReload.
	Reload time.Duration
	// OnLoad, if set, is called after each attempt to (re)load the file. On
	// an error the previous monitors keep running.
	OnLoad func(f *File, err error)

	mu      sync.Mutex
	targets []Target
}

// Targets returns the monitors currently running, in file order.
func (s *Supervisor) Targets() []Target {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.targets)
}

// Run runs until ctx is done.
func (s *Supervisor) Run(ctx context.Context) {
	every := s.Reload
	if every <= 0 {
		every = DefaultReload
	}
	for {
		stamp := Stamp(s.Path)
		f, err := Load(s.Path)
		if err == nil {
			names := map[string]bool{}
			for _, t := range f.Monitors {
				names[t.Name] = true
			}
			s.Runner.Tracker.Forget(names)
			s.mu.Lock()
			s.targets = f.Monitors
			s.mu.Unlock()
		}
		if s.OnLoad != nil {
			s.OnLoad(f, err)
		}
		if !s.runUntilChanged(ctx, s.Targets(), stamp, every) {
			return
		}
	}
}

// runUntilChanged runs targets until the file changes (true) or ctx ends (false).
func (s *Supervisor) runUntilChanged(ctx context.Context, targets []Target, stamp string, every time.Duration) bool {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Runner.Run(runCtx, targets)
	}()
	defer func() { cancel(); <-done }()
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-tick.C:
			if Stamp(s.Path) != stamp {
				return true
			}
		}
	}
}

// Stamp identifies a version of a file by its size and modification time;
// "missing" if it can't be read.
func Stamp(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return "missing"
	}
	return fmt.Sprintf("%d/%d", st.Size(), st.ModTime().UnixNano())
}
