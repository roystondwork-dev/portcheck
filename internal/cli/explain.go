package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/cmdports"
	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/freeport"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// explainTimeout bounds `explain`: it runs before the shell draws its prompt.
const explainTimeout = time.Second

// maxExplained bounds how many in-use ports `explain` reports.
const maxExplained = 3

// quietExits are exit codes that never mean "port in use": interrupted
// (130), suspended (148), not executable (126) or not found (127).
var quietExits = []int{0, 126, 127, 130, 148}

func (c *cli) newExplainCmd() *cobra.Command {
	var exit int
	cmd := &cobra.Command{
		Use:   "explain -- <command line>",
		Short: "Say who holds the port a failed command wanted (used by the shell hook)",
		Long: `Explain why a command line may have failed with "address already in use".

explain reads the ports the command line listens on (--port 3000, -p 8080:80,
PORT=4000, 0.0.0.0:8000, …) or, without one, the default port of a known dev
server (vite: 5173, rails server: 3000, flask run: 5000, …), expanding package
scripts such as 'npm run dev' from ./package.json. When something holds one of
those ports, it says what and how to free it. Otherwise it prints nothing.

The shell hook ('portcheck init') runs it after a command fails. It never
fails itself: errors and timeouts (1s) print nothing and exit 0.`,
		Example: `  portcheck explain --exit 1 -- 'npm run dev'
  portcheck explain -- 'python3 -m http.server 8000' --json`,
		Args: usageArgs(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.runExplain(cmd, strings.Join(args, " "), exit)
		},
	}
	cmd.Flags().IntVar(&exit, "exit", 1, "the command's exit status; 0, 126, 127, 130 and 148 print nothing")
	return cmd
}

func (c *cli) runExplain(cmd *cobra.Command, line string, exit int) error {
	data := output.ExplainJSON{Command: line, ExitCode: exit}
	var inUse []freeport.Status
	var lines []string
	if !slices.Contains(quietExits, exit) && !isPortcheck(line) {
		cands := cmdports.Ports(line, c.packageScripts())
		for _, cd := range cands {
			data.Ports = append(data.Ports, output.CandidateJSON{Port: cd.Port, Source: string(cd.Source)})
		}
		inUse = c.portsInUse(cmd.Context(), cands)
		for _, st := range inUse {
			lines = append(lines, output.ExplainLines(st, runHint(line, st.Port, cands))...)
		}
	}
	if len(lines) == 0 && !c.jsonOutput() {
		return nil // nothing to say: the common case
	}
	if err := c.printer.Print(output.ExplainDoc(data, inUse, lines)); err != nil {
		c.log.Debug("explain: print", "err", err)
	}
	return nil
}

// portsInUse checks the candidate ports and returns those something holds,
// at most maxExplained. Errors and timeouts count as "nothing to report".
func (c *cli) portsInUse(ctx context.Context, cands []cmdports.Candidate) []freeport.Status {
	if len(cands) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, explainTimeout)
	defer cancel()
	ranges := make([]portspec.Range, len(cands))
	for i, cd := range cands {
		ranges[i] = portspec.Range{Start: cd.Port, End: cd.Port}
	}
	sts, err := c.app.Ports.CheckAll(ctx, portspec.New(ranges...), inspect.TCP)
	if err != nil {
		c.log.Debug("explain: check", "err", err)
		return nil
	}
	var out []freeport.Status
	for _, st := range sts {
		if st.Availability == freeport.InUse && len(out) < maxExplained {
			out = append(out, st)
		}
	}
	return out
}

// jsonOutput reports whether --json / --output json was given.
func (c *cli) jsonOutput() bool {
	return c.jsonFlag || c.outputFlag == "json"
}

// packageScripts reads the "scripts" of ./package.json, once, when a command
// line runs one (npm run dev).
func (c *cli) packageScripts() cmdports.Scripts {
	var scripts map[string]string
	loaded := false
	return func(name string) string {
		if !loaded {
			loaded = true
			scripts = c.readScripts()
		}
		return scripts[name]
	}
}

func (c *cli) readScripts() map[string]string {
	wd, err := c.app.Workdir()
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(wd, "package.json")) //nolint:gosec // G304: the user's own project file, read-only
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return nil
	}
	return pkg.Scripts
}

// isPortcheck reports whether the command line runs portcheck itself.
func isPortcheck(line string) bool {
	first, _, _ := strings.Cut(strings.TrimSpace(line), " ")
	name := strings.TrimSuffix(filepath.Base(first), ".exe")
	return name == "portcheck" || name == "portcheck-cli"
}

// runHint suggests rerunning line on a free port with `portcheck run`, when the
// port was given explicitly and can be swapped for {port}.
func runHint(line string, port uint16, cands []cmdports.Candidate) string {
	explicit := slices.ContainsFunc(cands, func(cd cmdports.Candidate) bool {
		return cd.Port == port && cd.Source == cmdports.Explicit
	})
	if !explicit {
		return ""
	}
	re := regexp.MustCompile(`(^|[^0-9])` + strconv.Itoa(int(port)) + `([^0-9]|$)`)
	loc := re.FindStringSubmatchIndex(line)
	if loc == nil {
		return ""
	}
	// loc[3] ends the prefix group, loc[4] starts the suffix group.
	if strings.HasSuffix(line[:loc[3]], "=") && !strings.Contains(line[:loc[3]], "-") {
		return "" // PORT=3000 cmd: run starts programs without a shell, so no assignments
	}
	templated := line[:loc[3]] + "{port}" + line[loc[4]:]
	return fmt.Sprintf("portcheck run --port %d --next -- %s", port, templated)
}
