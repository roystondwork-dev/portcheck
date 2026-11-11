package monitor

import (
	"context"
	"sync"
	"time"
)

// Runner checks monitors on their schedules and reports results and events.
type Runner struct {
	// Check runs one check, e.g. (*Checker).Check.
	Check func(ctx context.Context, t Target) Result
	// Tracker turns results into events; required.
	Tracker *Tracker
	// OnResult and OnEvent, if set, are called after each check and for each
	// event. They may be called from several goroutines at once.
	OnResult func(Result)
	OnEvent  func(Event)
	// Interval overrides each target's schedule (tests); nil means
	// Target.Interval.
	Interval func(Target) time.Duration
}

// Run checks every target now and then on its schedule, until ctx is done.
// Targets must be valid.
func (r *Runner) Run(ctx context.Context, targets []Target) {
	var wg sync.WaitGroup
	for _, t := range targets {
		wg.Go(func() { r.loop(ctx, t) })
	}
	wg.Wait()
}

// CheckAll checks every target once, in parallel, and returns the results in
// the targets' order.
func (r *Runner) CheckAll(ctx context.Context, targets []Target) []Result {
	results := make([]Result, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Go(func() { results[i] = r.once(ctx, t) })
	}
	wg.Wait()
	return results
}

func (r *Runner) loop(ctx context.Context, t Target) {
	every := r.interval(t)
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		r.once(ctx, t)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (r *Runner) once(ctx context.Context, t Target) Result {
	res := r.Check(ctx, t)
	if ctx.Err() != nil {
		return res // shutting down: a cancelled check says nothing about the service
	}
	if r.OnResult != nil {
		r.OnResult(res)
	}
	if ev, ok := r.Tracker.Observe(res); ok && r.OnEvent != nil {
		r.OnEvent(ev)
	}
	return res
}

func (r *Runner) interval(t Target) time.Duration {
	if r.Interval != nil {
		return r.Interval(t)
	}
	d, _ := t.Interval()
	return d
}
