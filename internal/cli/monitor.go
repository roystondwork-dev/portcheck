package cli

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/monitor"
)

// defaultMonitorReload is how often `monitor run` looks for changes to the
// monitors file.
const defaultMonitorReload = monitor.DefaultReload

func (c *cli) newMonitorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Watch URLs and ports, and warn when one goes down",
		Long: `Keep an eye on services you depend on: URLs that must answer with a good
HTTP status (a /ping or /health endpoint, a website) and ports that must accept
connections (a database, a dev server).

Monitors run on this device only, while it's on: from 'portcheck monitor run',
the tray app (portcheck-tray) or the VS Code extension. Nothing is hosted, so
checks stop when the device sleeps or shuts down. A service is reported down
after 2 failed checks in a row, once, and again when it's back up.

Monitors are kept in monitors.yaml in portcheck's configuration folder.`,
		Example: `  portcheck monitor add https://example.com/health
  portcheck monitor add https://api.example.com/ping --every 30s --contains pong
  portcheck monitor add localhost:5432 --name postgres
  portcheck monitor check            # check all once
  portcheck monitor run              # keep checking, with desktop notifications`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error { return c.runMonitorList(cmd) },
	}
	cmd.AddCommand(c.newMonitorAddCmd(), c.newMonitorListCmd(), c.newMonitorRemoveCmd(), c.newMonitorCheckCmd(), c.newMonitorRunCmd())
	return cmd
}

// monitorsPath is the monitors file in portcheck's configuration folder.
func (c *cli) monitorsPath() (string, error) {
	dir, err := c.configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, monitor.FileName), nil
}

func (c *cli) loadMonitors() (string, *monitor.File, error) {
	path, err := c.monitorsPath()
	if err != nil {
		return "", nil, err
	}
	f, err := monitor.Load(path)
	if err != nil {
		return path, nil, fmt.Errorf("%w; fix the file or remove it", err)
	}
	return path, f, nil
}

type monitorAddFlags struct {
	name, every, timeout, status, contains string
}

func (c *cli) newMonitorAddCmd() *cobra.Command {
	var f monitorAddFlags
	cmd := &cobra.Command{
		Use:   "add <url|host:port|port>",
		Short: "Add a monitor",
		Long: `Add a URL (http or https) or a TCP address to monitor, and check it once.

A URL is up when it answers within --timeout with a status in --status
(default 200-299, after redirects) and, with --contains, a body containing
that text. An address is up when it accepts a connection. A bare port means
localhost.`,
		Example: `  portcheck monitor add https://example.com/health
  portcheck monitor add https://api.example.com/ping --every 30s --contains pong
  portcheck monitor add db.internal:5432 --name postgres --every 5m`,
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error { return c.runMonitorAdd(cmd, args[0], f) },
	}
	fl := cmd.Flags()
	fl.StringVar(&f.name, "name", "", "name in lists and alerts (default: the URL without https://, or the address)")
	fl.StringVar(&f.every, "every", "", "how often to check, at least 10s (default 1m)")
	fl.StringVar(&f.timeout, "timeout", "", "how long one check may take (default 10s)")
	fl.StringVar(&f.status, "status", "", "HTTP statuses that count as up, e.g. 200-399 or 200,204 (default 200-299)")
	fl.StringVar(&f.contains, "contains", "", "text the response must contain, e.g. pong")
	return cmd
}

func (c *cli) runMonitorAdd(cmd *cobra.Command, arg string, f monitorAddFlags) error {
	t, err := monitor.ParseTarget(arg)
	if err != nil {
		return usageError("%v", err)
	}
	if f.name != "" {
		t.Name = f.name
	}
	t.Every, t.Timeout, t.Status, t.Contains = f.every, f.timeout, f.status, f.contains
	path, file, err := c.loadMonitors()
	if err != nil {
		return err
	}
	if err := file.Add(t); err != nil {
		return usageError("%v", err)
	}
	if err := monitor.Save(path, file); err != nil {
		return err
	}
	every, _ := t.Interval()
	fmt.Fprintf(c.app.Stdout, "Added %s: checks %s every %s.\n", t.Name, t.Endpoint(), output.ShortDuration(every))
	r := c.app.MonitorCheck(cmd.Context(), t)
	if r.Up {
		fmt.Fprintf(c.app.Stdout, "It's up now (%.0f ms).\n", r.Latency)
	} else {
		fmt.Fprintf(c.app.Stderr, "portcheck: %s is down right now: %s\n", t.Name, r.Error)
	}
	fmt.Fprintln(c.app.Stdout, "To be warned when it goes down, keep 'portcheck monitor run' or the tray app running.")
	return nil
}

func (c *cli) newMonitorListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List monitors",
		Args:    usageArgs(cobra.NoArgs),
		RunE:    func(cmd *cobra.Command, _ []string) error { return c.runMonitorList(cmd) },
	}
}

func (c *cli) runMonitorList(*cobra.Command) error {
	path, file, err := c.loadMonitors()
	if err != nil {
		return err
	}
	return c.printer.Print(output.MonitorsDoc(path, file.Monitors))
}

func (c *cli) newMonitorRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name|url|address>",
		Aliases: []string{"rm"},
		Short:   "Remove a monitor",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(_ *cobra.Command, args []string) error {
			path, file, err := c.loadMonitors()
			if err != nil {
				return err
			}
			t, ok := file.Remove(args[0])
			if !ok {
				return usageError("no monitor named %q; see portcheck monitor list", args[0])
			}
			if err := monitor.Save(path, file); err != nil {
				return err
			}
			fmt.Fprintf(c.app.Stdout, "Removed %s.\n", t.Name)
			return nil
		},
	}
}

func (c *cli) newMonitorCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check [name...]",
		Short: "Check monitors once",
		Long: `Check every monitor (or the named ones) once, in parallel. One failed check
counts as down here. Exit status is 0 when all are up and 1 when any is down.`,
		RunE: func(cmd *cobra.Command, args []string) error { return c.runMonitorCheck(cmd, args) },
	}
}

func (c *cli) runMonitorCheck(cmd *cobra.Command, names []string) error {
	_, file, err := c.loadMonitors()
	if err != nil {
		return err
	}
	targets, err := pickMonitors(file.Monitors, names)
	if err != nil {
		return err
	}
	r := monitor.Runner{Check: c.app.MonitorCheck, Tracker: &monitor.Tracker{Threshold: 1}}
	results := r.CheckAll(cmd.Context(), targets)
	if err := c.printer.Print(output.MonitorCheckDoc(results)); err != nil {
		return err
	}
	for _, res := range results {
		if !res.Up {
			return negative()
		}
	}
	return nil
}

func pickMonitors(all []monitor.Target, names []string) ([]monitor.Target, error) {
	if len(names) == 0 {
		return all, nil
	}
	byName := map[string]monitor.Target{}
	for _, t := range all {
		byName[t.Name] = t
	}
	picked := make([]monitor.Target, 0, len(names))
	for _, n := range names {
		t, ok := byName[n]
		if !ok {
			return nil, usageError("no monitor named %q; see portcheck monitor list", n)
		}
		picked = append(picked, t)
	}
	return picked, nil
}

type monitorRunFlags struct {
	threshold int
	noNotify  bool
	reload    time.Duration
}

func (c *cli) newMonitorRunCmd() *cobra.Command {
	var f monitorRunFlags
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Keep checking monitors and warn when one goes down",
		Long: `Check every monitor on its schedule until stopped (Ctrl-C). When a service
goes down (after --threshold failed checks in a row) or comes back up, print a
line and show a desktop notification. Changes to the monitors file are picked
up while running.

This runs only while this terminal (and the device) is on. For checks in the
background, use the tray app, or start this command at login.`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error { return c.runMonitorRun(cmd, f) },
	}
	fl := cmd.Flags()
	fl.IntVar(&f.threshold, "threshold", monitor.DefaultThreshold, "failed checks in a row before a service counts as down")
	fl.BoolVar(&f.noNotify, "no-notify", false, "print alerts only; no desktop notifications")
	fl.DurationVar(&f.reload, "reload-every", defaultMonitorReload, "how often to look for changes to the monitors file")
	_ = fl.MarkHidden("reload-every")
	return cmd
}

func (c *cli) runMonitorRun(cmd *cobra.Command, f monitorRunFlags) error {
	if f.threshold < 1 {
		return usageError("--threshold must be at least 1")
	}
	if f.reload <= 0 {
		return usageError("--reload-every must be positive")
	}
	ctx := cmd.Context()
	tracker := &monitor.Tracker{Threshold: f.threshold}
	var notifyFailed atomic.Bool
	runner := &monitor.Runner{
		Check:   c.app.MonitorCheck,
		Tracker: tracker,
		OnResult: func(r monitor.Result) {
			c.log.Debug("monitor check", "name", r.Name, "up", r.Up, "latencyMs", r.Latency, "error", r.Error)
		},
		OnEvent: func(e monitor.Event) {
			word := "UP  "
			if e.Status == monitor.Down {
				word = "DOWN"
			}
			fmt.Fprintf(c.app.Stdout, "%s  %s  %s\n", e.At.Local().Format("2006-01-02 15:04:05"), word, e.Message())
			if f.noNotify {
				return
			}
			if err := c.app.Notify.Notify(ctx, "portcheck", e.Message()); err != nil && notifyFailed.CompareAndSwap(false, true) {
				// Say it once, then keep printing.
				fmt.Fprintf(c.app.Stderr, "portcheck: %v (alerts are still printed here)\n", err)
			}
		},
	}
	// Load once first, so a broken file is an error rather than a warning.
	path, _, err := c.loadMonitors()
	if err != nil {
		return err
	}
	loaded := false // OnLoad runs on this goroutine, one call at a time
	sup := &monitor.Supervisor{
		Path:   path,
		Runner: runner,
		Reload: f.reload,
		OnLoad: func(file *monitor.File, err error) {
			if loaded {
				fmt.Fprintln(c.app.Stderr, "Monitors changed; reloading.")
			}
			loaded = true
			switch {
			case err != nil:
				fmt.Fprintf(c.app.Stderr, "portcheck: %v; still using the previous monitors\n", err)
			case len(file.Monitors) == 0:
				fmt.Fprintln(c.app.Stderr, "No monitors yet; add one with 'portcheck monitor add <url>'. Waiting for changes…")
			default:
				fmt.Fprintf(c.app.Stderr, "Monitoring %s (Ctrl-C to stop)…\n", plural(len(file.Monitors), "service"))
			}
		},
	}
	sup.Run(ctx)
	return nil // stopped (Ctrl-C)
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
