package cli

import (
	"cmp"
	"fmt"
	"maps"
	"os/user"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
)

type serversFlags struct {
	olderThan string
	all       bool
	stop      bool
	kill      killFlags
}

func (c *cli) newServersCmd() *cobra.Command {
	var f serversFlags
	cmd := &cobra.Command{
		Use:   "servers",
		Short: "List the dev servers you started, with their project and uptime",
		Long: `List your own processes that listen on a TCP port and run inside a project
folder, oldest first: dev servers, databases and tools you started from a
terminal or editor and may have forgotten about.

The project is the nearest folder at or above the process's working
directory that holds .git, package.json, go.mod or a similar file. Apps that
run from / or your home folder (AirPlay, Spotify, …) are left out unless you
pass --all.

With --stop, the listed servers are stopped after confirmation, with the same
safety guards as 'portcheck kill'.`,
		Example: `  portcheck servers
  portcheck servers --older-than 1d
  portcheck servers --older-than 12h --stop
  portcheck servers --all --json`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error { return c.runServers(cmd, f) },
	}
	fl := cmd.Flags()
	fl.StringVar(&f.olderThan, "older-than", "", "only servers running at least this long, e.g. 30m, 12h, 2d")
	fl.BoolVar(&f.all, "all", false, "also include your processes that run outside a project folder")
	fl.BoolVar(&f.stop, "stop", false, "stop the listed servers (asks first)")
	fl.BoolVarP(&f.kill.yes, "yes", "y", false, "with --stop: do not ask for confirmation")
	fl.BoolVar(&f.kill.dryRun, "dry-run", false, "with --stop: show what would be stopped")
	fl.DurationVar(&f.kill.grace, "grace", killer.DefaultGrace, "with --stop: how long to wait for a graceful exit")
	return cmd
}

func (c *cli) runServers(cmd *cobra.Command, f serversFlags) error {
	minAge, err := parseAge(f.olderThan)
	if err != nil {
		return usageError("%v", err)
	}
	if !f.stop && (f.kill.yes || f.kill.dryRun) {
		return usageError("--yes and --dry-run only apply with --stop")
	}
	if f.kill.grace <= 0 {
		return usageError("--grace must be positive")
	}
	me, err := c.app.CurrentUser()
	if err != nil {
		return fmt.Errorf("current user: %w", err)
	}
	res, err := c.app.Lister.List(cmd.Context(), inspect.Filter{Protos: []inspect.Proto{inspect.TCP}, ListeningOnly: true})
	if err != nil {
		return err
	}
	servers := collectServers(res.Sockets, me, f.all, c.app.Now(), minAge)
	if f.stop {
		if len(servers) == 0 {
			fmt.Fprintln(c.app.Stderr, "No servers to stop.")
			return negative()
		}
		targets := make([]killTarget, len(servers))
		for i, s := range servers {
			anc := make([]string, len(s.Ancestors))
			for j, a := range s.Ancestors {
				anc[j] = a.Name + " (" + strconv.Itoa(int(a.PID)) + ")"
			}
			targets[i] = killTarget{pid: s.PID, name: s.Name, ports: s.Ports, project: s.Project, ancestry: strings.Join(anc, " ← ")}
		}
		return c.stopTargets(cmd.Context(), targets, f.kill)
	}
	if err := c.printer.Print(output.ServersDoc(servers)); err != nil {
		return err
	}
	if len(servers) == 0 {
		return &ExitError{Code: ExitNegative, Silent: true}
	}
	return nil
}

// collectServers groups listening sockets by process and keeps the current
// user's processes in a project (all of them with all), oldest first.
func collectServers(socks []inspect.Socket, me string, all bool, now time.Time, minAge time.Duration) []output.ServerJSON {
	byPID := map[int32]*output.ServerJSON{}
	for _, s := range socks {
		p := s.Process
		if p == nil || s.Container != nil || !sameUser(p.User, me) || (p.Project == "" && !all) {
			continue
		}
		srv := byPID[p.PID]
		if srv == nil {
			srv = &output.ServerJSON{ProcessJSON: output.ProcessInfoJSON(p)}
			if !p.CreateTime.IsZero() {
				srv.UptimeSeconds = int64(now.Sub(p.CreateTime).Seconds())
			}
			byPID[p.PID] = srv
		}
		if !slices.Contains(srv.Ports, s.Port()) {
			srv.Ports = append(srv.Ports, s.Port())
		}
	}
	out := make([]output.ServerJSON, 0, len(byPID))
	for _, pid := range slices.Sorted(maps.Keys(byPID)) {
		srv := byPID[pid]
		if minAge > 0 && (srv.StartTime == nil || time.Duration(srv.UptimeSeconds)*time.Second < minAge) {
			continue
		}
		slices.Sort(srv.Ports)
		out = append(out, *srv)
	}
	// Oldest first: the ones most likely forgotten. Unknown start times last.
	slices.SortStableFunc(out, func(a, b output.ServerJSON) int {
		if (a.StartTime == nil) != (b.StartTime == nil) {
			if a.StartTime == nil {
				return 1
			}
			return -1
		}
		return cmp.Compare(b.UptimeSeconds, a.UptimeSeconds)
	})
	return out
}

// sameUser compares a process owner with the current user. Windows reports
// names as DOMAIN\user, and they are case-insensitive there.
func sameUser(owner, me string) bool {
	if owner == "" || me == "" {
		return false
	}
	short := func(name string) string {
		_, after, found := strings.Cut(name, `\`)
		if found {
			return after
		}
		return name
	}
	return strings.EqualFold(short(owner), short(me))
}

// parseAge parses --older-than: a Go duration (90m, 12h) or whole days (2d).
func parseAge(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid --older-than %q: use e.g. 30m, 12h or 2d", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid --older-than %q: use e.g. 30m, 12h or 2d", s)
	}
	return d, nil
}

func currentUser() (string, error) { return userName(user.Current) }

// userName returns the name of the user that lookup finds.
func userName(lookup func() (*user.User, error)) (string, error) {
	u, err := lookup()
	if err != nil {
		return "", fmt.Errorf("look up user: %w", err)
	}
	return u.Username, nil
}
