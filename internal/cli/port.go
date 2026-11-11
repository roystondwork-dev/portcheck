package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/output"
	"github.com/roystondwork-dev/portcheck/internal/portreg"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/portspec"
)

// defaultProjectRange is where `portcheck port` assigns project ports.
const defaultProjectRange = "3000-3999"

// configDirEnv overrides where portcheck keeps its files.
const configDirEnv = "PORTCHECK_CONFIG_DIR"

type portFlags struct {
	portRange string
	list      bool
	forget    bool
	set       int
	force     bool
}

func (c *cli) newPortCmd() *cobra.Command {
	var f portFlags
	cmd := &cobra.Command{
		Use:   "port",
		Short: "This project's own port, the same every time",
		Long: `Print the port this project uses on this machine, assigning one the first time.

The project is the nearest folder at or above the current directory that holds
.git, package.json, go.mod or a similar file. Each project gets its own port
from --range, starting from a hash of its path so it tends to be the same on
every machine, and skipping ports that are in use or belong to another project.
The assignment is remembered, so the port never changes unless you --set or
--forget it.

The plain output is just the number, for scripts:

  PORT=$(portcheck port) npm run dev
  portcheck run --port auto -- npm run dev    # the same, and explains a busy port

Assignments are kept in ports.json in portcheck's configuration folder
(` + "$" + configDirEnv + ` overrides it). Ports shared with your team belong in
.portcheck.yaml instead: see 'portcheck doctor'.`,
		Example: `  portcheck port
  PORT=$(portcheck port) npm run dev
  portcheck port --list
  portcheck port --set 3005
  portcheck port --forget`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error { return c.runPort(cmd, f) },
	}
	fl := cmd.Flags()
	fl.StringVar(&f.portRange, "range", defaultProjectRange, "ports to assign from")
	fl.BoolVar(&f.list, "list", false, "list every project's port")
	fl.BoolVar(&f.forget, "forget", false, "remove this project's port")
	fl.IntVar(&f.set, "set", 0, "give this project this port")
	fl.BoolVar(&f.force, "force", false, "with --set: take the port from the project that has it")
	return cmd
}

func (c *cli) runPort(cmd *cobra.Command, f portFlags) error {
	setGiven := cmd.Flags().Changed("set")
	modes := 0
	for _, on := range []bool{f.list, f.forget, setGiven} {
		if on {
			modes++
		}
	}
	switch {
	case modes > 1:
		return usageError("--list, --forget and --set can't be combined")
	case f.force && !setGiven:
		return usageError("--force only applies to --set")
	case modes > 0 && cmd.Flags().Changed("range"):
		return usageError("--range only applies when assigning a port")
	case setGiven && (f.set < 1 || f.set > math.MaxUint16):
		return usageError("--set must be between 1 and 65535")
	}
	reg, err := c.registry()
	if err != nil {
		return err
	}
	switch {
	case f.list:
		return c.listPorts(reg)
	case f.forget:
		return c.forgetPort(reg)
	case setGiven:
		return c.setPort(cmd.Context(), reg, uint16(f.set), f.force)
	}
	spec, err := portspec.Parse(f.portRange)
	if err != nil {
		return &ExitError{Code: ExitUsage, Err: err}
	}
	p, err := c.projectPort(cmd.Context(), reg, spec)
	if err != nil {
		return err
	}
	if !p.Assigned {
		c.noteBusy(cmd.Context(), p)
	}
	return c.printer.Print(output.PortDoc(p))
}

// configDir is portcheck's configuration folder: $PORTCHECK_CONFIG_DIR, or a
// portcheck folder in the user's configuration directory.
func (c *cli) configDir() (string, error) {
	if dir := envValue(c.app.Environ, configDirEnv); dir != "" {
		return dir, nil
	}
	base, err := c.app.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("find the configuration folder: %w (set %s)", err, configDirEnv)
	}
	return filepath.Join(base, "portcheck"), nil
}

// registry opens the port registry in portcheck's configuration folder.
func (c *cli) registry() (*portreg.Registry, error) {
	dir, err := c.configDir()
	if err != nil {
		return nil, err
	}
	return portreg.New(dir, func(format string, a ...any) {
		fmt.Fprintf(c.app.Stderr, "portcheck: "+format+"\n", a...)
	}), nil
}

// currentProject is the project the working directory is in: its root and name.
func (c *cli) currentProject() (root, name string, err error) {
	wd, err := c.app.Workdir()
	if err != nil {
		return "", "", fmt.Errorf("working directory: %w", err)
	}
	root = inspect.ProjectRoot(wd)
	if root == "" {
		return "", "", usageError("%s is not a project folder; run this in your project's directory", wd)
	}
	// One key per project, however the path was reached (e.g. /tmp vs /private/tmp).
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	return root, filepath.Base(root), nil
}

// projectPort returns the current project's port, assigning one from spec the
// first time, and records that it was used.
func (c *cli) projectPort(ctx context.Context, reg *portreg.Registry, spec portspec.Spec) (output.PortJSON, error) {
	root, name, err := c.currentProject()
	if err != nil {
		return output.PortJSON{}, err
	}
	res := output.PortJSON{Project: name, Path: root}
	err = reg.Update(func(f *portreg.File) error {
		e, ok := f.Projects[root]
		if !ok {
			port, err := portreg.Assign(f, root, spec, func(p uint16) (bool, error) { return c.portFree(ctx, p) })
			if err != nil {
				return err
			}
			e = portreg.Entry{Name: name, Port: port}
			res.Assigned = true
		}
		e.LastUsed = c.app.Now().UTC()
		f.Projects[root] = e
		res.Port = e.Port
		return nil
	})
	if errors.Is(err, portreg.ErrNoPort) {
		return res, &ExitError{Code: ExitNegative, Err: fmt.Errorf("can't assign a port to %s: %w (try a wider --range)", name, err)}
	}
	if err != nil {
		return res, err
	}
	if res.Assigned {
		fmt.Fprintf(c.app.Stderr, "portcheck: %s's port is now %d (%s)\n", name, res.Port, root)
	}
	return res, nil
}

func (c *cli) portFree(ctx context.Context, p uint16) (bool, error) {
	sts, err := c.app.Ports.CheckAll(ctx, portspec.Single(p), inspect.TCP)
	if err != nil {
		return false, err
	}
	return sts[0].Free(), nil
}

// noteBusy tells the user, on stderr, when the project's port is in use.
func (c *cli) noteBusy(ctx context.Context, p output.PortJSON) {
	sts, err := c.app.Ports.CheckAll(ctx, portspec.Single(p.Port), inspect.TCP)
	if err == nil && !sts[0].Free() {
		fmt.Fprintf(c.app.Stderr, "portcheck: note: %s's port %d is %s\n", p.Project, p.Port, describeBusy(sts[0]))
	}
}

func (c *cli) listPorts(reg *portreg.Registry) error {
	f, err := reg.Read()
	if err != nil {
		return err
	}
	entries := make([]output.PortEntryJSON, 0, len(f.Projects))
	for root, e := range f.Projects {
		_, statErr := os.Stat(root)
		entries = append(entries, output.PortEntryJSON{Port: e.Port, Project: e.Name, Path: root, LastUsed: e.LastUsed, Exists: statErr == nil})
	}
	slices.SortFunc(entries, func(a, b output.PortEntryJSON) int {
		return cmp.Or(cmp.Compare(a.Port, b.Port), strings.Compare(a.Path, b.Path))
	})
	return c.printer.Print(output.PortsDoc(entries))
}

func (c *cli) forgetPort(reg *portreg.Registry) error {
	root, name, err := c.currentProject()
	if err != nil {
		return err
	}
	var gone portreg.Entry
	found := false
	err = reg.Update(func(f *portreg.File) error {
		gone, found = f.Projects[root]
		delete(f.Projects, root)
		return nil
	})
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintf(c.app.Stderr, "%s has no port assigned.\n", name)
		return negative()
	}
	return c.printer.Print(output.PortDoc(output.PortJSON{Port: gone.Port, Project: name, Path: root, Forgotten: true}))
}

func (c *cli) setPort(ctx context.Context, reg *portreg.Registry, port uint16, force bool) error {
	root, name, err := c.currentProject()
	if err != nil {
		return err
	}
	var owner string
	err = reg.Update(func(f *portreg.File) error {
		if other, taken := f.Owner(port, root); taken {
			if !force {
				owner = other
				return errPortTaken
			}
			delete(f.Projects, other)
		}
		f.Projects[root] = portreg.Entry{Name: name, Port: port, LastUsed: c.app.Now().UTC()}
		return nil
	})
	if errors.Is(err, errPortTaken) {
		return &ExitError{Code: ExitNegative, Err: fmt.Errorf("port %d belongs to %s; pass --force to move it to %s", port, owner, name)}
	}
	if err != nil {
		return err
	}
	p := output.PortJSON{Port: port, Project: name, Path: root, Assigned: true}
	c.noteBusy(ctx, p)
	return c.printer.Print(output.PortDoc(p))
}

var errPortTaken = errors.New("port assigned to another project")

// envValue returns name's value in environ, "" if unset.
func envValue(environ []string, name string) string {
	for _, kv := range slices.Backward(environ) {
		if k, v, ok := strings.Cut(kv, "="); ok && k == name {
			return v
		}
	}
	return ""
}

// autoPort is the --port value that means "this project's port".
const autoPort = "auto"

// resolveAutoPort is `run --port auto`: the project's port, assigned from
// --range if given (else the project range) the first time.
func (c *cli) resolveAutoPort(cmd *cobra.Command, f runFlags) (uint16, error) {
	rng := defaultProjectRange
	if cmd.Flags().Changed("range") {
		rng = f.portRange
	}
	spec, err := portspec.Parse(rng)
	if err != nil {
		return 0, &ExitError{Code: ExitUsage, Err: err}
	}
	reg, err := c.registry()
	if err != nil {
		return 0, err
	}
	p, err := c.projectPort(cmd.Context(), reg, spec)
	if err != nil {
		return 0, err
	}
	return p.Port, nil
}
