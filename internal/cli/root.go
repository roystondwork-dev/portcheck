package cli

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// cli holds global flag values and per-invocation state shared by commands.
type cli struct {
	app *App

	outputFlag string
	jsonFlag   bool
	noColor    bool
	verbose    bool
	logFormat  string

	printer *output.Printer
	log     *slog.Logger
}

// NewRootCmd builds the portcheck command tree. It is exported for the
// documentation generator; programs should use Execute or Main.
func NewRootCmd(app *App) *cobra.Command {
	c := &cli{app: app}
	root := &cobra.Command{
		Use:   "portcheck [port...]",
		Short: "See what's using your ports, find free ones, and free them up",
		Long: `portcheck shows which processes own local ports, checks whether ports are
free, finds free ports, safely stops the process holding a port, and checks
ports on remote hosts.

Running 'portcheck' alone in an interactive terminal opens the live TUI
('portcheck tui'); 'portcheck <port>' is a shortcut for 'portcheck check <port>'.`,
		Example: `  portcheck                       # live interactive view (in a terminal)
  portcheck list --listening      # everything accepting connections
  portcheck 3000                  # is 3000 free? who holds it?
  portcheck free -n 3             # three free ports
  portcheck kill 8080             # stop whatever listens on 8080
  portcheck scan db.internal -p 5432,6379
  portcheck wait 5432 -t 60s      # block until Postgres accepts connections
  portcheck run --port 3000 --next -- npm start`,
		Version:           app.Build.Version,
		DisableAutoGenTag: true, // keep generated docs reproducible
		Args:              cobra.ArbitraryArgs,
		SilenceErrors:     true,
		SilenceUsage:      true,
		PersistentPreRunE: c.setup,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				if c.app.StdinTTY && c.app.StdoutTTY {
					return c.runTUI(cmd, defaultTUIInterval)
				}
				return cmd.Help()
			}
			if _, err := portspec.Parse(strings.Join(args, ",")); err != nil {
				msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
				if s := cmd.SuggestionsFor(args[0]); len(s) > 0 {
					msg += "\n\nDid you mean this?\n\t" + strings.Join(s, "\n\t")
				}
				return usageError("%s", msg)
			}
			return c.runCheck(cmd, args, inspect.TCP)
		},
	}
	root.SuggestionsMinimumDistance = 2
	root.SetIn(app.Stdin)
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	root.SetVersionTemplate("{{.Version}}\n")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &ExitError{Code: ExitUsage, Err: err}
	})

	pf := root.PersistentFlags()
	pf.StringVarP(&c.outputFlag, "output", "o", "", "output format: table, plain, csv, json (default: table on a terminal, plain otherwise)")
	pf.BoolVar(&c.jsonFlag, "json", false, "shorthand for --output json")
	pf.BoolVar(&c.noColor, "no-color", false, "disable colors (also honors NO_COLOR)")
	pf.BoolVarP(&c.verbose, "verbose", "v", false, "log diagnostic details to stderr")
	pf.StringVar(&c.logFormat, "log-format", "text", "log format: text or json")

	root.AddCommand(
		c.newListCmd(),
		c.newCheckCmd(),
		c.newFreeCmd(),
		c.newKillCmd(),
		c.newScanCmd(),
		c.newTUICmd(),
		c.newWaitCmd(),
		c.newRunCmd(),
		c.newDoctorCmd(),
		c.newBrowserCmd(),
		c.newMCPCmd(),
		c.newVersionCmd(),
	)
	return root
}

// setup validates global flags and builds the printer and logger.
func (c *cli) setup(cmd *cobra.Command, _ []string) error {
	format := output.Plain
	if c.app.StdoutTTY {
		format = output.Table
	}
	switch {
	case c.jsonFlag && c.outputFlag != "" && c.outputFlag != string(output.JSON):
		return usageError("--json conflicts with --output %s", c.outputFlag)
	case c.jsonFlag:
		format = output.JSON
	case c.outputFlag != "":
		f, err := output.ParseFormat(c.outputFlag)
		if err != nil {
			return &ExitError{Code: ExitUsage, Err: err}
		}
		format = f
	}
	c.printer = output.NewPrinter(cmd.OutOrStdout(), format, output.Options{NoColor: c.noColor, Environ: c.app.Environ})

	level := slog.LevelWarn
	if c.verbose {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	switch c.logFormat {
	case "text":
		h = slog.NewTextHandler(c.app.Stderr, opts)
	case "json":
		h = slog.NewJSONHandler(c.app.Stderr, opts)
	default:
		return usageError("unknown --log-format %q (want text or json)", c.logFormat)
	}
	c.log = slog.New(h)
	return nil
}

// hintPrivileges prints the OS-specific privilege hint, if any.
func (c *cli) hintPrivileges() {
	if h := c.app.PrivilegeHint(); h != "" {
		fmt.Fprintln(c.app.Stderr, "hint: "+h)
	}
}

func parseProtos(values []string) ([]inspect.Proto, error) {
	out := make([]inspect.Proto, 0, len(values))
	for _, v := range values {
		p, err := parseProto(v)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func parseProto(v string) (inspect.Proto, error) {
	switch p := inspect.Proto(strings.ToLower(strings.TrimSpace(v))); p {
	case inspect.TCP, inspect.UDP:
		return p, nil
	default:
		return "", usageError("unknown protocol %q (want tcp or udp)", v)
	}
}
